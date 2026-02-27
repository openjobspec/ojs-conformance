// Command portal runs the OJS Conformance Certification Portal.
//
// It serves the portal HTTP API for certification requests, certificate
// management, badge generation, and verification.
//
// Usage:
//
//	go run ./cmd/portal
//	go run ./cmd/portal -addr :8090
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openjobspec/ojs-conformance/badge"
)

func main() {
	addr := ":8090"
	if v := os.Getenv("PORTAL_ADDR"); v != "" {
		addr = v
	}
	if len(os.Args) > 2 && os.Args[1] == "-addr" {
		addr = os.Args[2]
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	portal := badge.NewPortal()
	mux := http.NewServeMux()
	portal.RegisterRoutes(mux)

	// Health endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"ojs-conformance-portal"}`))
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("portal starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.String("error", err.Error()))
	}

	fmt.Println("portal stopped")
}
