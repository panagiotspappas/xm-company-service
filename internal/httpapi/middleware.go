package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

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
				logger.InfoContext(
					request.Context(),
					"http request completed",
					slog.String("request_id", requestID),
					slog.String("method", request.Method),
					slog.String("path", request.URL.Path),
					slog.Int("status", response.finalStatus()),
					slog.Duration("duration", time.Since(started)),
				)
			}()

			next.ServeHTTP(response, request)
		})
	}
}
