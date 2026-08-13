package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/panagiotspappas/xm-company-service/internal/httpapi"
)

const (
	readHeaderTimeout         = 5 * time.Second
	readTimeout               = 10 * time.Second
	applicationRequestTimeout = 15 * time.Second
	writeTimeout              = 20 * time.Second
	idleTimeout               = 60 * time.Second
	shutdownGracePeriod       = 20 * time.Second
)

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

func newHTTPServer(address string, router http.Handler, logger *slog.Logger) *http.Server {
	timeoutMiddleware := httpapi.RequestTimeout(applicationRequestTimeout)
	timeoutHandler := timeoutMiddleware(router)

	recoveryMiddleware := httpapi.RecoverPanics(logger)
	recoveryHandler := recoveryMiddleware(timeoutHandler)

	loggingMiddleware := httpapi.RequestLogger(logger)
	loggingHandler := loggingMiddleware(recoveryHandler)

	requestIDMiddleware := httpapi.RequestID(uuid.New)
	handler := requestIDMiddleware(loggingHandler)

	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

func serveHTTP(
	server httpServer,
	shutdownContext context.Context,
	stopSignals func(),
	logger *slog.Logger,
	gracePeriod time.Duration,
) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		return normalizeServeError(err)
	case <-shutdownContext.Done():
		stopSignals()
	}

	ctx, cancel := context.WithTimeout(context.Background(), gracePeriod)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("HTTP server graceful shutdown failed", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return fmt.Errorf("force close HTTP server: %w", closeErr)
		}
	}

	return normalizeServeError(<-serveResult)
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return fmt.Errorf("serve HTTP: %w", err)
}
