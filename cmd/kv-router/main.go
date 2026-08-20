// Package main provides the entrypoint for kv-router, a KV-cache-aware
// request router for LLM inference backends.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/arinmallanna/kv-router/internal/api"
	"github.com/arinmallanna/kv-router/internal/backend"
	"github.com/arinmallanna/kv-router/internal/cacheindex"
	"github.com/arinmallanna/kv-router/internal/config"
	"github.com/arinmallanna/kv-router/internal/scorer"
	"github.com/arinmallanna/kv-router/internal/telemetry"
	"github.com/arinmallanna/kv-router/internal/tokenizer"
)

const version = "0.1.0"

func main() {
	os.Exit(run())
}

func run() int {
	// --- Flag parsing ---
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	listenOverride := flag.String("listen", "", "override listen address (e.g. :9090)")
	flag.Parse()

	// --- Structured logger ---
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// --- Load config ---
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err, "path", *configPath)
		return 1
	}

	if *listenOverride != "" {
		cfg.ListenAddr = *listenOverride
	}

	// --- Initialize tokenizer (block hasher) ---
	blockHasher := &tokenizer.BlockHasher{
		BlockSize: cfg.BlockSize,
	}

	// --- Initialize bounded, authoritative cache-residency directory ---
	cacheDirectory := cacheindex.New()

	// --- Initialize backend pool ---
	poolConfigs := make([]backend.BackendConfig, len(cfg.Backends))
	for i, b := range cfg.Backends {
		poolConfigs[i] = backend.BackendConfig{
			ID:                  b.ID,
			URL:                 b.URL,
			CacheCapacityBlocks: b.CacheCapacityBlocks,
			HealthCheckInterval: b.HealthCheckInterval,
			MaxConcurrent:       b.MaxConcurrent,
		}
		cacheDirectory.Register(b.ID, b.CacheCapacityBlocks)
	}
	pool := backend.NewPool(poolConfigs)

	// --- Initialize scorer ---
	weights := scorer.Weights{
		CacheHit:     cfg.Scorer.CacheHitWeight,
		QueueDepth:   cfg.Scorer.QueueDepthWeight,
		EvictionRisk: cfg.Scorer.EvictionRiskWeight,
	}
	routeScorer := scorer.New(weights)
	routeTelemetry := telemetry.New(1000)

	// --- Build API server ---
	srv := api.NewServer(
		cfg.ListenAddr,
		blockHasher,
		cacheDirectory,
		routeScorer,
		pool,
		routeTelemetry,
		poolConfigs,
	)

	// --- Context for lifecycle management ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Start health checks in background ---
	go pool.StartHealthChecks(ctx)

	// --- Startup banner ---
	slog.Info("kv-router starting",
		"version", version,
		"listen", cfg.ListenAddr,
		"backends", len(cfg.Backends),
		"block_size", cfg.BlockSize,
		"scorer_weights", weights,
	)

	// --- Graceful shutdown on signal ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Block until signal or server error.
	select {
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig.String())
		cancel()
		if err := <-errCh; err != nil {
			slog.Error("shutdown error", "error", err)
			return 1
		}
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
			return 1
		}
	}

	slog.Info("shutdown complete")
	return 0
}
