package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

// RequestIDGenerator generates request identifiers.
type RequestIDGenerator func() uuid.UUID

// RequestID attaches a valid request ID to every request and response.
func RequestID(generate RequestIDGenerator) Middleware {
	if generate == nil {
		panic("httpapi: request ID generator is required")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("httpapi: request ID handler is required")
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requestID := incomingRequestID(request)
			if requestID == uuid.Nil {
				requestID = generate()
				if requestID == uuid.Nil {
					panic("httpapi: request ID generator returned a nil UUID")
				}
			}

			canonicalRequestID := requestID.String()
			writer.Header().Set(requestIDHeader, canonicalRequestID)
			ctx := context.WithValue(
				request.Context(),
				requestIDContextKey{},
				canonicalRequestID,
			)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

// RequestIDFromContext returns the canonical request ID stored in ctx.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDContextKey{}).(string)
	return requestID, ok && requestID != ""
}

func incomingRequestID(request *http.Request) uuid.UUID {
	values := request.Header.Values(requestIDHeader)
	if len(values) != 1 {
		return uuid.Nil
	}

	requestID, err := uuid.Parse(values[0])
	if err != nil || requestID == uuid.Nil {
		return uuid.Nil
	}

	return requestID
}
