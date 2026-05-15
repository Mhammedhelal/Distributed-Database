package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	masterauth "github.com/distributed-db/master/internal/auth"
	"github.com/distributed-db/master/config"
	"github.com/distributed-db/master/internal/api"
	"github.com/distributed-db/master/internal/cluster"
	"github.com/distributed-db/master/internal/db"
	"github.com/distributed-db/master/internal/election"
	"github.com/distributed-db/master/internal/query"
	"github.com/distributed-db/master/internal/replication"
	"github.com/distributed-db/master/internal/wal"
)

func main() {
	cfgPath := flag.String("config", "config/master.json", "path to master.json")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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

	// ── Storage ───────────────────────────────────────────────────────────────
	store, err := db.New(cfg.MySQL.DSN)
	if err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}
	defer store.Close()

	// ── WAL ───────────────────────────────────────────────────────────────────
	if err := os.MkdirAll("/data", 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	walLog, err := wal.Open(cfg.WAL.Path)
	if err != nil {
		return fmt.Errorf("open wal: %w", err)
	}
	defer walLog.Close()

	// ── Cluster registry ──────────────────────────────────────────────────────
	registry := cluster.NewRegistry(cfg.HeartbeatInterval(), cfg.Cluster.MissThreshold, logger)
	for i, addr := range cfg.Cluster.WorkerAddresses {
		registry.Register(cluster.Node{
			ID:      fmt.Sprintf("%d", i+2),
			Address: addr,
			Role:    cluster.RoleWorker,
			Status:  cluster.StatusAlive,
		})
	}

	// ── Election ──────────────────────────────────────────────────────────────
	electionMgr, err := election.New(cfg.Cluster.NodeID, cfg.Cluster.SelfAddress, registry, logger)
	if err != nil {
		return fmt.Errorf("init election: %w", err)
	}
	electionMgr.SetMaster(true)

	// ── Replicator ────────────────────────────────────────────────────────────
	signer := masterauth.New(cfg.Auth.HMACSecret)
	replicator := replication.New(registry, signer, logger)

	// ── Query engine ──────────────────────────────────────────────────────────
	executor := query.NewExecutor(store, true)

	// ── HTTP API ──────────────────────────────────────────────────────────────
	handler := api.New(executor, walLog, replicator, registry, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.HealthHandler)
	mux.HandleFunc("POST /query", handler.QueryHandler)
	mux.HandleFunc("GET /cluster/status", handler.ClusterStatusHandler)
	mux.HandleFunc("GET /internal/wal", handler.WALSinceHandler)
	mux.HandleFunc("POST /internal/heartbeat", registry.HeartbeatHandler)
	mux.HandleFunc("POST /internal/election", electionMgr.ElectionHandler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("master listening", "addr", addr, "node_id", cfg.Cluster.NodeID)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case sig := <-quit:
		logger.Info("shutdown", "signal", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}