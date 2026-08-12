package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/panagiotspappas/xm-company-service/internal/auth"
	"github.com/panagiotspappas/xm-company-service/internal/company"
	"github.com/panagiotspappas/xm-company-service/internal/config"
	"github.com/panagiotspappas/xm-company-service/internal/httpapi"
	postgresrepository "github.com/panagiotspappas/xm-company-service/internal/postgres"
)

const databaseStartupTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("company service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	configureLogger(cfg)

	startupContext, cancelStartup := context.WithTimeout(context.Background(), databaseStartupTimeout)
	defer cancelStartup()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse PostgreSQL pool configuration: %w", err)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns

	pool, err := pgxpool.NewWithConfig(startupContext, poolConfig)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(startupContext); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	slog.Info("database connection established")

	repository := postgresrepository.NewRepository(pool)
	service := company.NewService(repository, uuid.NewRandom)
	tokenValidator := auth.NewValidator(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.Audience)
	handler := httpapi.NewRouter(
		service,
		httpapi.RequireAuthentication(tokenValidator),
		pool,
	)
	server := &http.Server{
		Addr:    cfg.HTTPAddress,
		Handler: handler,
	}

	slog.Info("HTTP server starting", "address", cfg.HTTPAddress)
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}

	return nil
}

func configureLogger(cfg config.Config) {
	options := &slog.HandlerOptions{
		Level:     cfg.LogLevel,
		AddSource: true,
	}

	var handler slog.Handler
	if cfg.LogFormat == config.LogFormatJSON {
		handler = slog.NewJSONHandler(os.Stdout, options)
	} else {
		handler = slog.NewTextHandler(os.Stdout, options)
	}

	slog.SetDefault(slog.New(handler))
}
