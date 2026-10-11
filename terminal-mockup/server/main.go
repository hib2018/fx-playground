package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	staticDir := flag.String("static", "", "optional static asset directory (for example ../web/dist)")
	flag.Parse()
	workspace, err := os.Getwd()
	if err != nil {
		log.Fatal("cannot determine workspace")
	}
	server := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           newServer(workspace, os.Getenv("AI_GATEWAY_API_KEY"), *staticDir),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 * 1024,
		// No write timeout: gateway responses may be long-lived streams.
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Print("mock server listening on http://127.0.0.1:8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server stopped unexpectedly")
	}
}
