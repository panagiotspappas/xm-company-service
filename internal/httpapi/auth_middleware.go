package httpapi

import (
	"net/http"
	"strings"
)

// TokenValidator defines the token verification behavior consumed by the HTTP API.
type TokenValidator interface {
	Validate(string) error
}

// Middleware decorates an HTTP handler.
type Middleware func(http.Handler) http.Handler

// RequireAuthentication requires a valid Bearer token before invoking next.
func RequireAuthentication(validator TokenValidator) Middleware {
	if validator == nil {
		panic("httpapi: token validator is required")
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			panic("httpapi: authenticated handler is required")
		}

		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			authorizationValues := request.Header.Values("Authorization")
			if len(authorizationValues) != 1 {
				writeUnauthorized(writer)
				return
			}

			fields := strings.Fields(authorizationValues[0])
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
				writeUnauthorized(writer)
				return
			}

			if err := validator.Validate(fields[1]); err != nil {
				writeUnauthorized(writer)
				return
			}

			next.ServeHTTP(writer, request)
		})
	}
}

func writeUnauthorized(writer http.ResponseWriter) {
	writer.Header().Set("WWW-Authenticate", "Bearer")
	writeError(writer, http.StatusUnauthorized, errorCodeUnauthorized, "unauthorized")
}
