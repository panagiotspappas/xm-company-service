package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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
	logger := slog.Default()

	startupContext, cancelStartup := context.WithTimeout(context.Background(), databaseStartupTimeout)
	defer cancelStartup()

	databaseStarted := time.Now()
	pool, err := openDatabase(startupContext, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	slog.Info("database connection established", "duration", time.Since(databaseStarted), "max_conns", cfg.DatabaseMaxConns)

	repository := postgresrepository.NewRepository(pool)
	service := company.NewService(repository, uuid.NewRandom)
	tokenValidator := auth.NewValidator(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.Audience)
	router := httpapi.NewRouter(
		service,
		httpapi.RequireAuthentication(tokenValidator),
		pool,
	)
	server := newHTTPServer(cfg.HTTPAddress, router, logger)

	signalContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	logger.Info("HTTP server starting", "address", cfg.HTTPAddress)
	if err := serveHTTP(
		server,
		signalContext,
		stopSignals,
		logger,
		shutdownGracePeriod,
	); err != nil {
		return err
	}

	return nil
}

func openDatabase(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL pool configuration: %w", err)
	}

	poolConfig.MaxConns = cfg.DatabaseMaxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return pool, nil
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
