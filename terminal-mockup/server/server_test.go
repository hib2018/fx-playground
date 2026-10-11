package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func request(s *Server, method, path, body, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStatusAndTasks(t *testing.T) {
	s := newServer("/test/workspace", "", "")
	status := decode[map[string]any](t, request(s, "GET", "/api/status", "", ""))
	if status["gatewayConfigured"] != false || status["workspace"] != "/test/workspace" || status["mode"] != "mock" {
		t.Fatal(status)
	}
	tasks := decode[[]Task](t, request(s, "GET", "/api/tasks", "", ""))
	if len(tasks) != 3 || tasks[0].Title == "" || tasks[0].Description == "" || tasks[0].Tag == "" {
		t.Fatal(tasks)
	}
	for _, want := range []string{"doing", "done", "todo"} {
		w := request(s, "POST", "/api/tasks/1/advance", "", "")
		if w.Code != 200 || decode[Task](t, w).Status != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if w := request(s, "POST", "/api/tasks/missing/advance", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/tasks/1/advance", "", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
}

func TestConcurrentRotate(t *testing.T) {
	s := newServer("mock", "", "")
	var wg sync.WaitGroup
	for i := 0; i < 301; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := request(s, "POST", "/api/tasks/1/advance", "", ""); w.Code != 200 {
				t.Errorf("advance: %d", w.Code)
			}
			if w := request(s, "GET", "/api/tasks", "", ""); w.Code != 200 {
				t.Errorf("list: %d", w.Code)
			}
		}()
	}
	wg.Wait()
	tasks := decode[[]Task](t, request(s, "GET", "/api/tasks", "", ""))
	if tasks[0].Status != "doing" {
		t.Fatal(tasks[0])
	}
}

func TestMockCommands(t *testing.T) {
	s := newServer("mock", "", "")
	for command, stdout := range mockCommands {
		body, _ := json.Marshal(map[string]string{"command": command})
		w := request(s, "POST", "/api/workspace/exec", string(body), "")
		got := decode[commandResult](t, w)
		if w.Code != 200 || got.Stdout != stdout || got.Stderr != "" || got.ExitCode != 0 {
			t.Fatalf("%s: %+v", command, got)
		}
	}
	for _, command := range []string{"rm -rf /", "pwd; ls", " ls", "cat /etc/passwd", "go test ./... && pwd"} {
		body, _ := json.Marshal(map[string]string{"command": command})
		got := decode[commandResult](t, request(s, "POST", "/api/workspace/exec", string(body), ""))
		if got.ExitCode != 127 || got.Stdout != "" || got.Stderr == "" {
			t.Fatal(got)
		}
	}
	for _, body := range []string{"{", `{"command":"pwd"} {}`, `{"command":42}`, `{"command":"pwd","other":true}`} {
		if w := request(s, "POST", "/api/workspace/exec", body, ""); w.Code != 400 {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	if w := request(s, "POST", "/api/workspace/exec", strings.Repeat("x", maxBody+1), ""); w.Code != 413 {
		t.Fatal(w.Code)
	}
}

func gatewayPath(path string) string { return "/api/gateway?path=" + url.QueryEscape(path) }

func TestGatewayValidation(t *testing.T) {
	s := newServer("mock", "", "")
	tests := []struct {
		method, path string
		code         int
	}{
		{"GET", gatewayPath("/coding-agent/v1/models"), 503},
		{"POST", gatewayPath("/v3/ai/language-model"), 503},
		{"POST", gatewayPath("/coding-agent/v1/models"), 405},
		{"GET", gatewayPath("/v3/ai/language-model"), 405},
		{"PUT", gatewayPath("/v3/ai/language-model"), 405},
		{"GET", gatewayPath("https://example.com"), 400},
		{"GET", gatewayPath("/coding-agent/v1/models?extra=1"), 400},
		{"GET", gatewayPath("/coding-agent/v1/../v1/models"), 400},
		{"GET", "/api/gateway", 400},
		{"GET", gatewayPath("/coding-agent/v1/models") + "&path=/v3/ai/language-model", 400},
		{"GET", "/api/gateway?path=%zz", 400},
	}
	for _, tt := range tests {
		if w := request(s, tt.method, tt.path, "", ""); w.Code != tt.code {
			t.Errorf("%s %s: %d want %d", tt.method, tt.path, w.Code, tt.code)
		}
	}
}

func TestOrigins(t *testing.T) {
	s := newServer("mock", "", "")
	for _, origin := range []string{"", "http://127.0.0.1:5173", "http://localhost:5173", "http://127.0.0.1:8080"} {
		if w := request(s, "POST", "/api/tasks/1/advance", "", origin); w.Code != 200 {
			t.Errorf("%q: %d", origin, w.Code)
		}
	}
	for _, origin := range []string{"null", "https://evil.example", "http://localhost:9999", "http://127.0.0.1:5173.evil.example", "http://127.0.0.1:8080/", "https://127.0.0.1:8080"} {
		for _, path := range []string{"/api/tasks/1/advance", "/api/workspace/exec", gatewayPath("/v3/ai/language-model"), "/api/status"} {
			if w := request(s, "POST", path, `{"command":"pwd"}`, origin); w.Code != 403 {
				t.Errorf("%q %s: %d", origin, path, w.Code)
			}
		}
	}
	r := httptest.NewRequest("POST", "http://localhost:8080/api/tasks/1/advance", nil)
	r.Header.Set("Origin", "http://localhost:8080")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	r.Header.Add("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r.Header.Del("Origin")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// All gateway tests use an in-memory transport, never the real gateway.
func TestGatewayForwarding(t *testing.T) {
	s := newServer("mock", "server-key", "")
	officialHeaders := map[string]string{
		"ai-language-model-id":        "test-language-model",
		"ai-language-model-streaming": "true",
		"ai-gateway-protocol-version": "0.0.1",
	}
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		for name, want := range officialHeaders {
			if got := r.Header.Get(name); got != want {
				t.Errorf("upstream %s = %q, want %q", name, got, want)
			}
		}
		if r.URL.String() != gatewayURL+"/v3/ai/language-model" || r.Method != "POST" {
			t.Fatal(r.URL, r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer server-key" || r.Header.Get("Ai-Model") != "test-model" {
			t.Fatal("incorrect credential/model forwarding")
		}
		for _, name := range []string{"Cookie", "Ai-Secret", "X-Api-Key"} {
			if r.Header.Get(name) != "" {
				t.Fatal("unexpected forwarded header", name)
			}
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != `{"prompt":"mock"}` || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal(string(data))
		}
		headers := http.Header{"Content-Type": {"text/event-stream"}, "Ai-Protocol-Version": {"1"}, "Set-Cookie": {"secret"}, "Ai-Secret": {"secret"}}
		for name, value := range officialHeaders {
			headers.Set(name, value)
		}
		return &http.Response{StatusCode: 201, Header: headers, Body: io.NopCloser(strings.NewReader("data: mock\n\n"))}, nil
	})
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080"+gatewayPath("/v3/ai/language-model"), strings.NewReader(`{"prompt":"mock"}`))
	r.Header.Set("Authorization", "Bearer client-key")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Ai-Model", "test-model")
	for name, value := range officialHeaders {
		r.Header.Set(name, value)
	}
	r.Header.Set("Ai-Secret", "secret")
	r.Header.Set("Cookie", "secret")
	r.Header.Set("X-Api-Key", "secret")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 201 || !w.Flushed || w.Body.String() != "data: mock\n\n" || w.Header().Get("Ai-Protocol-Version") != "1" || w.Header().Get("Set-Cookie") != "" {
		t.Fatal(w)
	}
	for name, want := range officialHeaders {
		if got := w.Header().Get(name); got != want {
			t.Errorf("downstream %s = %q, want %q", name, got, want)
		}
	}
	if w.Header().Get("Ai-Secret") != "" {
		t.Error("non-allowlisted response header forwarded")
	}
	if w := request(s, "POST", gatewayPath("/v3/ai/language-model"), strings.Repeat("x", maxBody+1), ""); w.Code != 413 {
		t.Fatal(w.Code)
	}
}

func TestGatewayTimeout(t *testing.T) {
	s := newServer("mock", "server-key", "")
	if s.gatewayTimeout != 2*time.Minute {
		t.Fatalf("default timeout = %v", s.gatewayTimeout)
	}
	// Exercise the production deadline path without waiting two minutes.
	s.gatewayTimeout = 20 * time.Millisecond
	var upstreamContext context.Context
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamContext = r.Context()
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > s.gatewayTimeout {
			t.Fatal("missing or unbounded upstream deadline")
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(2 * time.Second):
			t.Error("upstream timeout did not fire")
			return nil, context.DeadlineExceeded
		}
	})
	w := request(s, "GET", gatewayPath("/coding-agent/v1/models"), "", "")
	if w.Code != http.StatusGatewayTimeout || upstreamContext.Err() != context.DeadlineExceeded {
		t.Fatalf("status %d, upstream error %v", w.Code, upstreamContext.Err())
	}
	if strings.Contains(w.Body.String(), "server-key") {
		t.Fatal("error response exposed credentials")
	}
}

func TestGatewayCancellation(t *testing.T) {
	s := newServer("mock", "server-key", "")
	started := make(chan struct{})
	canceled := make(chan struct{})
	s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(canceled)
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+gatewayPath("/coding-agent/v1/models"), nil).WithContext(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.ServeHTTP(httptest.NewRecorder(), r) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not cancel")
	}
	<-done
}

func TestStaticFlagHandler(t *testing.T) {
	s := newServer("mock", "", t.TempDir())
	if w := request(s, "GET", "/", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/unknown", "", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
