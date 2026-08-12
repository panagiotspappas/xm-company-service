package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

const clientClosedRequestStatus = 499

type statusResponseWriter struct {
	http.ResponseWriter
	status    int
	committed bool
}

func (writer *statusResponseWriter) WriteHeader(status int) {
	if writer.committed {
		return
	}

	writer.status = status
	writer.committed = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusResponseWriter) Write(body []byte) (int, error) {
	if !writer.committed {
		writer.WriteHeader(http.StatusOK)
	}

	return writer.ResponseWriter.Write(body)
}

func (writer *statusResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (writer *statusResponseWriter) finalStatus() int {
	if !writer.committed {
		return http.StatusOK
	}

	return writer.status
}

// RequestLogger logs one structured completion record for every request.
func RequestLogger(logger *slog.Logger) Middleware {
	if logger == nil {
		panic("httpapi: request logger is required")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("httpapi: request logging handler is required")
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			response := &statusResponseWriter{ResponseWriter: writer}
			started := time.Now()

			defer func() {
				requestID, _ := RequestIDFromContext(request.Context())
				status := response.finalStatus()
				if !response.committed && request.Context().Err() == context.Canceled {
					status = clientClosedRequestStatus
				}
				logger.InfoContext(
					request.Context(),
					"http request completed",
					slog.String("request_id", requestID),
					slog.String("method", request.Method),
					slog.String("path", request.URL.Path),
					slog.Int("status", status),
					slog.Duration("duration", time.Since(started)),
				)
			}()

			next.ServeHTTP(response, request)
		})
	}
}

// RecoverPanics converts downstream panics into generic internal errors when
// the response has not already been committed.
func RecoverPanics(logger *slog.Logger) Middleware {
	if logger == nil {
		panic("httpapi: panic logger is required")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("httpapi: panic recovery handler is required")
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			response := &statusResponseWriter{ResponseWriter: writer}

			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				requestID, _ := RequestIDFromContext(request.Context())
				logger.ErrorContext(
					request.Context(),
					"http request panic",
					slog.String("request_id", requestID),
					slog.Any("panic", recovered),
					slog.String("stack", string(debug.Stack())),
				)

				if response.committed {
					panic(http.ErrAbortHandler)
				}

				writeError(
					response,
					http.StatusInternalServerError,
					errorCodeInternal,
					"internal server error",
				)
			}()

			next.ServeHTTP(response, request)
		})
	}
}

// RequestTimeout bounds downstream request work with a derived context.
func RequestTimeout(timeout time.Duration) Middleware {
	if timeout <= 0 {
		panic("httpapi: request timeout must be positive")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("httpapi: request timeout handler is required")
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			ctx, cancel := context.WithTimeout(request.Context(), timeout)
			defer cancel()

			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}
