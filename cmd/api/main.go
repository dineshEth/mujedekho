// cmd/api runs the REST API server. Monitoring execution lives in cmd/worker;
// this binary only serves customer traffic and reads the database.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"site24x7/internal/api"
	"site24x7/internal/auth"
	"site24x7/internal/config"
	"site24x7/internal/observability"
	"site24x7/internal/safety"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
	"site24x7/internal/worker"
)

func main() {
	config.LoadDotEnv()
	if err := run(); err != nil {
		observability.NewLogger("error").Error("api: fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(level(cfg.Env))
	if cfg.DatabaseURL == "" {
		return errors.New("api: DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return errors.New("api: JWT_SECRET must be at least 32 bytes (provide one even in development)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	all, err := store.NewAll(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer all.Close()

	tokens := auth.NewTokens([]byte(cfg.JWTSecret), cfg.AccessTTL)
	validator := safety.NewValidator(cfg.DNSResolvers, 5*time.Second)

	handler := api.NewRouter(&api.API{
		All:    all,
		Tokens: tokens,
		Probe:  worker.NewHTTPProbe(validator),
		Up:     uptime.New(all.Pool),
		Log:    log,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		host := cfg.HTTPAddr
		if strings.HasPrefix(host, ":") {
			host = "localhost" + host
		}
		log.Info(fmt.Sprintf("server started on http://%s", host), "url", "http://"+host, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("api: shutdown requested")
	}

	shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Error("api: graceful shutdown failed", "err", err)
	}
	log.Info("api: stopped cleanly")
	return nil
}

func level(env string) string {
	if env == "development" {
		return "debug"
	}
	return "info"
}
