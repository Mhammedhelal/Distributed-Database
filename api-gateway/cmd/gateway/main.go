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

	"api-gateway/config"
	"api-gateway/internal/auth"
	"api-gateway/internal/health"
	"api-gateway/internal/ratelimit"
	"api-gateway/internal/router"
)

func main() {
	cfgPath := flag.String("config", "config/gateway.json", "path to gateway.json")
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
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	signer := auth.NewSigner(cfg.Auth.HMACSecret, cfg.Auth.TokenTTL.Duration)
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

	mux := chi.NewRouter()
	mux.Use(chimid.RequestID)
	mux.Use(structuredLogger(logger))
	mux.Use(chimid.Recoverer)
	mux.Use(auth.StripExternalMasterToken)
	mux.Use(limiter.Middleware)
	mux.Use(auth.ClientAuth(cfg.Auth.ClientAPIKey))

	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"api-gateway"}`))
	})
	mux.Get("/cluster/status", healthProxy.Handler())
	mux.Handle("/*", rt)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout.Duration,
		WriteTimeout: cfg.Server.WriteTimeout.Duration,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("gateway stopped cleanly")
	return nil
}

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
