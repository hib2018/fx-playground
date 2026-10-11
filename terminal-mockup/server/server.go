package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxBody = 1 << 20
const gatewayURL = "https://ai-gateway.vercel.sh"

type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Tag         string `json:"tag"`
}

type Server struct {
	mu             sync.Mutex
	tasks          []Task
	workspace      string
	key            string
	client         *http.Client
	static         http.Handler
	gatewayTimeout time.Duration
}

func newServer(workspace, key, staticDir string) *Server {
	s := &Server{
		workspace: workspace, key: key,
		gatewayTimeout: 2 * time.Minute,
		client:         &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		tasks: []Task{
			{"1", "Inspect workspace", "Review the mock workspace and README.", "todo", "setup"},
			{"2", "Build terminal UI", "Connect the terminal UI to the local API.", "doing", "frontend"},
			{"3", "Verify mock commands", "Check the canned command responses.", "done", "test"},
		},
	}
	if staticDir != "" {
		s.static = http.FileServer(http.Dir(staticDir))
	}
	return s
}

func jsonResponse(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, code int, message string) {
	jsonResponse(w, code, map[string]string{"error": message})
}

// Requests without Origin are permitted for local non-browser clients. No CORS
// headers are emitted: browsers should use the Vite same-origin proxy.
func allowedOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	if len(origins) != 1 {
		return false
	}
	origin := origins[0]
	if origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.Host == r.Host && (u.Host == "127.0.0.1:8080" || u.Host == "localhost:8080")
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !allowedOrigin(r) {
		apiError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	switch {
	case r.URL.Path == "/api/status":
		if !method(w, r, http.MethodGet) {
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{"gatewayConfigured": s.key != "", "workspace": s.workspace, "mode": "mock"})
	case r.URL.Path == "/api/tasks":
		if !method(w, r, http.MethodGet) {
			return
		}
		s.mu.Lock()
		tasks := append([]Task(nil), s.tasks...)
		s.mu.Unlock()
		jsonResponse(w, http.StatusOK, tasks)
	case strings.HasPrefix(r.URL.Path, "/api/tasks/") && strings.HasSuffix(r.URL.Path, "/advance"):
		if !method(w, r, http.MethodPost) {
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/advance")
		s.mu.Lock()
		var found *Task
		for i := range s.tasks {
			if s.tasks[i].ID == id {
				switch s.tasks[i].Status {
				case "todo":
					s.tasks[i].Status = "doing"
				case "doing":
					s.tasks[i].Status = "done"
				default:
					s.tasks[i].Status = "todo"
				}
				copy := s.tasks[i]
				found = &copy
				break
			}
		}
		s.mu.Unlock()
		if found == nil {
			apiError(w, http.StatusNotFound, "task not found")
			return
		}
		jsonResponse(w, http.StatusOK, found)
	case r.URL.Path == "/api/workspace/exec":
		if !method(w, r, http.MethodPost) {
			return
		}
		s.exec(w, r)
	case r.URL.Path == "/api/gateway":
		s.gateway(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/") || s.static == nil {
			apiError(w, http.StatusNotFound, "not found")
			return
		}
		s.static.ServeHTTP(w, r)
	}
}

func method(w http.ResponseWriter, r *http.Request, expected string) bool {
	if r.Method == expected {
		return true
	}
	w.Header().Set("Allow", expected)
	apiError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

type commandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// These outputs are fixed mock data; commands never access the OS or shell.
var mockCommands = map[string]string{
	"pwd":           "/mock/workspace\n",
	"ls":            "README.md\nserver\nweb\n",
	"cat README.md": "# Terminal Mockup\n\nThis is a mock workspace.\n",
	"go test ./...": "ok  mock/workspace  0.001s\n",
	"git status":    "On branch main\nnothing to commit, working tree clean\n",
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			apiError(w, http.StatusRequestEntityTooLarge, "body too large")
		} else {
			apiError(w, http.StatusBadRequest, "invalid body")
		}
		return nil, false
	}
	return body, true
}

func (s *Server) exec(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var input struct {
		Command string `json:"command"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		apiError(w, http.StatusBadRequest, "invalid command JSON")
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		apiError(w, http.StatusBadRequest, "invalid command JSON")
		return
	}
	stdout, known := mockCommands[input.Command]
	result := commandResult{Stdout: stdout}
	if !known {
		result.Stderr = "mock: command not found\n"
		result.ExitCode = 127
	}
	jsonResponse(w, http.StatusOK, result)
}

// Explicit protocol/model header names only; client credentials, cookies, and
// arbitrary ai-* headers are never forwarded.
var protocolHeaders = []string{
	"Ai-Model", "Ai-Model-Id", "Ai-Protocol", "Ai-Protocol-Version",
	"Ai-Streaming-Protocol", "Ai-Language-Model-Specification-Version",
	"Ai-Language-Model-Id", "Ai-Language-Model-Streaming", "Ai-Gateway-Protocol-Version",
}

func copyProtocolHeaders(dst, src http.Header) {
	for _, name := range protocolHeaders {
		for _, value := range src.Values(name) {
			dst.Add(name, value)
		}
	}
}

func (s *Server) gateway(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["path"]) != 1 {
		apiError(w, http.StatusBadRequest, "invalid gateway path")
		return
	}
	path := query.Get("path")
	expected := ""
	switch path {
	case "/coding-agent/v1/models":
		expected = http.MethodGet
	case "/v3/ai/language-model":
		expected = http.MethodPost
	default:
		apiError(w, http.StatusBadRequest, "gateway path not allowed")
		return
	}
	if !method(w, r, expected) {
		return
	}
	if s.key == "" {
		apiError(w, http.StatusServiceUnavailable, "gateway not configured")
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// The deadline covers both response headers and the entire upstream stream,
	// while retaining cancellation from the browser's request context.
	ctx, cancel := context.WithTimeout(r.Context(), s.gatewayTimeout)
	defer cancel()
	upstream, err := http.NewRequestWithContext(ctx, r.Method, gatewayURL+path, bytes.NewReader(body))
	if err != nil {
		apiError(w, http.StatusBadGateway, "gateway request failed")
		return
	}
	upstream.Header.Set("Authorization", "Bearer "+s.key)
	for _, name := range []string{"Content-Type", "Accept"} {
		if value := r.Header.Get(name); value != "" {
			upstream.Header.Set(name, value)
		}
	}
	copyProtocolHeaders(upstream.Header, r.Header)
	response, err := s.client.Do(upstream)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			apiError(w, http.StatusGatewayTimeout, "gateway timed out")
		} else {
			apiError(w, http.StatusBadGateway, "gateway unavailable")
		}
		return
	}
	defer response.Body.Close()
	for _, name := range []string{"Content-Type", "Cache-Control"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	copyProtocolHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	controller := http.NewResponseController(w)
	_ = controller.Flush()
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}
