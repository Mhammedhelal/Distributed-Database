package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chi "github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"

	"github.com/distributed-db/api-gateway/config"
	"github.com/distributed-db/api-gateway/internal/auth"
	"github.com/distributed-db/api-gateway/internal/health"
	"github.com/distributed-db/api-gateway/internal/ratelimit"
	"github.com/distributed-db/api-gateway/internal/router"
)

func main() {
	cfgPath := flag.String("config", "config/gateway.yaml", "path to gateway.yaml")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := run(*cfgPath, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfgPath string, logger *slog.Logger) error {
	// ── Load configuration ────────────────────────────────────────────────────
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── Build dependencies ────────────────────────────────────────────────────
	signer := auth.NewSigner(cfg.Auth.HMACSecret, cfg.Auth.TokenTTL)
	limiter := ratelimit.New(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)

	workerURLs := make([]string, len(cfg.Nodes.Workers))
	for i, w := range cfg.Nodes.Workers {
		workerURLs[i] = w.Address
	}

	rt := router.New(router.Config{
		MasterURL:  cfg.Nodes.Master.Address,
		WorkerURLs: workerURLs,
		Signer:     signer,
		Logger:     logger,
	})

	workerRefs := make([]struct{ ID, Address string }, len(cfg.Nodes.Workers))
	for i, w := range cfg.Nodes.Workers {
		workerRefs[i] = struct{ ID, Address string }{w.ID, w.Address}
	}
	healthProxy := health.New(cfg.Nodes.Master.ID, cfg.Nodes.Master.Address, workerRefs)

	// ── Build chi router ──────────────────────────────────────────────────────
	mux := chi.NewRouter()

	// Global middleware — applied to every request in declaration order.
	mux.Use(chimid.RequestID)
	mux.Use(structuredLogger(logger))
	mux.Use(chimid.Recoverer)
	mux.Use(auth.StripExternalMasterToken) // MUST be first security middleware
	mux.Use(limiter.Middleware)
	mux.Use(auth.ClientAuth(cfg.Auth.ClientAPIKey))

	// Dedicated routes handled by the gateway itself.
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"api-gateway"}`)) //nolint:errcheck
	})
	mux.Get("/cluster/status", healthProxy.Handler())

	// Everything else is handled by the router (proxy logic).
	mux.Handle("/*", rt)

	// ── Start server ──────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
		}
		close(errCh)
	}()

	select {
	case sig := <-quit:
		logger.Info("shutting down", "signal", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("gateway stopped cleanly")
	return nil
}

// structuredLogger returns a chi middleware that emits a structured log line
// for every request.
func structuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimid.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"request_id", chimid.GetReqID(r.Context()),
					"remote", r.RemoteAddr,
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}