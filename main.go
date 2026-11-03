package main

import (
	"PeriCache/cache"
	"PeriCache/metrics"
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
	addr := flag.String("addr", envOr("CACHE_ADDR", ":8080"), "HTTP listen address")
	maxSize := flag.Int("max-size", 100, "maximum cache entries")
	cleanupSeconds := flag.Int("cleanup-interval", 30, "expired-entry cleanup interval in seconds")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	rec := metrics.NewMemoryRecorder()
	store := cache.NewMemoryStore()
	defer func() {
		if err := store.Close(context.Background()); err != nil {
			logger.Error("failed to close store", "error", err)
		}
	}()

	c, err := cache.NewCache(ctx, logger, *maxSize, rec, store)
	if err != nil {
		logger.Error("failed to create cache", "error", err)
		os.Exit(1)
	}

	if err := seedCache(ctx, c); err != nil {
		logger.Error("failed to seed cache", "error", err)
		os.Exit(1)
	}
	logger.Info("cache seeded", "entries", len(seedEntries), "ttl_seconds", seedTTLSeconds)

	if err := runServer(ctx, logger, c, *addr, *cleanupSeconds); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func runServer(ctx context.Context, logger *slog.Logger, c cache.Cache, addr string, cleanupSeconds int) error {
	cleanupDone := c.StartCleanup(ctx, cleanupSeconds)

	mux := http.NewServeMux()
	cache.RegisterRoutes(mux, c)

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
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown failed", "error", err)
			<-cleanupDone
			return err
		}
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
