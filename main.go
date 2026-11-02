package main

import (
	"Nandos-Pair-Coding/cache"
	"Nandos-Pair-Coding/metrics"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	mode := flag.String("mode", envOr("CACHE_MODE", "server"), "operation mode: server | demo")
	addr := flag.String("addr", envOr("CACHE_ADDR", ":8080"), "HTTP listen address")
	maxSize := flag.Int("max-size", 100, "maximum cache entries")
	cleanupSeconds := flag.Int("cleanup-interval", 30, "expired-entry cleanup interval in seconds")
	metricsBackend := flag.String("metrics", envOr("METRICS_BACKEND", "both"), "metrics backend: memory | prometheus | both")
	storeBackend := flag.String("store", envOr("CACHE_STORE", "memory"), "store backend: memory | mongo")
	mongoURI := flag.String("mongo-uri", envOr("MONGO_URI", "mongodb://localhost:27017"), "MongoDB connection URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "nandos_cache"), "MongoDB database name")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rec, err := metrics.NewFromBackend(*metricsBackend, nil)
	if err != nil {
		logger.Error("failed to create metrics recorder", "error", err)
		os.Exit(1)
	}

	store, err := cache.NewStore(ctx, *storeBackend, *mongoURI, *mongoDB)
	if err != nil {
		logger.Error("failed to create store", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = store.Close(context.Background())
	}()

	c, err := cache.NewCache(ctx, logger, *maxSize, rec, store)
	if err != nil {
		logger.Error("failed to create cache", "error", err)
		os.Exit(1)
	}

	logger.Info("cache configured", "store", *storeBackend, "metrics", *metricsBackend)

	switch *mode {
	case "demo":
		cache.Run(ctx, c)
	case "server":
		if err := runServer(ctx, logger, c, rec, *addr, *cleanupSeconds); err != nil {
			logger.Error("server stopped with error", "error", err)
			os.Exit(1)
		}
	default:
		logger.Error("unknown mode", "mode", *mode)
		os.Exit(1)
	}
}

func runServer(ctx context.Context, logger *slog.Logger, c cache.Cache, rec metrics.Recorder, addr string, cleanupSeconds int) error {
	cleanupDone := c.StartCleanup(ctx, cleanupSeconds)

	mux := http.NewServeMux()
	cache.RegisterRoutes(mux, c)
	if h := metrics.HTTPHandler(rec); h != nil {
		mux.Handle("GET /metrics", h)
		logger.Info("prometheus metrics enabled", "path", "/metrics")
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("cache HTTP server listening", "addr", addr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-cleanupDone
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
