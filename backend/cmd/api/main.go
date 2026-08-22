package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/handlers"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

func main() {
	cfg := config.Load()

	// In PR1, initialize in-memory store stub (Turso SQLite will be integrated in PR2)
	memStore := store.NewMemStore()

	// Create router with stdlib ServeMux (Go 1.22+) and middleware chain
	router := handlers.NewRouter(cfg, memStore)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server run context for graceful shutdown
	serverCtx, serverStopCtx := context.WithCancel(context.Background())

	// Listen for syscall signals for process to interrupt/quit
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-sig

		// Shutdown signal with grace period of 10 seconds
		shutdownCtx, cancel := context.WithTimeout(serverCtx, 10*time.Second)
		defer cancel()

		go func() {
			<-shutdownCtx.Done()
			if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
				log.Fatal("Graceful shutdown timed out... forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		log.Println("Shutting down server gracefully...")
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Server shutdown error: %v", err)
		}
		serverStopCtx()
	}()

	log.Printf("Ocicat Bella backend starting on port :%s (env: %s)", cfg.Port, cfg.Env)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server ListenAndServe failed: %v", err)
	}

	// Wait for server context to be stopped
	<-serverCtx.Done()
	fmt.Println("Server stopped cleanly.")
}
