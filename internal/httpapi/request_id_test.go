package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

const (
	generatedRequestID     = "ed8e0d83-c1dd-44e5-a79e-71185496034f"
	incomingRequestIDValue = "1d9db363-47f4-4d0b-b36f-bb351e4b8873"
)

func TestRequestIDAcceptsValidIncomingID(t *testing.T) {
	generatedCalls := 0
	handlerCalls := 0
	handler := RequestID(func() uuid.UUID {
		generatedCalls++
		return uuid.MustParse(generatedRequestID)
	})(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handlerCalls++
		assertContextRequestID(t, request.Context(), incomingRequestIDValue)
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/companies", nil)
	request.Header.Set(requestIDHeader, "{1D9DB363-47F4-4D0B-B36F-BB351E4B8873}")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got != incomingRequestIDValue {
		t.Fatalf("%s = %q, want %q", requestIDHeader, got, incomingRequestIDValue)
	}
	if generatedCalls != 0 {
		t.Fatalf("generator calls = %d, want 0", generatedCalls)
	}
	if handlerCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handlerCalls)
	}
}

func TestRequestIDGeneratesForInvalidIncomingID(t *testing.T) {
	tests := map[string][]string{
		"missing":   nil,
		"empty":     {""},
		"malformed": {"not-a-uuid"},
		"nil UUID":  {uuid.Nil.String()},
		"duplicate": {incomingRequestIDValue, generatedRequestID},
	}

	for name, requestIDValues := range tests {
		t.Run(name, func(t *testing.T) {
			generatedCalls := 0
			handlerCalls := 0
			handler := RequestID(func() uuid.UUID {
				generatedCalls++
				return uuid.MustParse(generatedRequestID)
			})(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				handlerCalls++
				assertContextRequestID(t, request.Context(), generatedRequestID)
				writer.WriteHeader(http.StatusUnauthorized)
			}))
			request := httptest.NewRequest(http.MethodPost, "/companies", nil)
			for _, value := range requestIDValues {
				request.Header.Add(requestIDHeader, value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if got := response.Header().Get(requestIDHeader); got != generatedRequestID {
				t.Fatalf("%s = %q, want %q", requestIDHeader, got, generatedRequestID)
			}
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if generatedCalls != 1 {
				t.Fatalf("generator calls = %d, want 1", generatedCalls)
			}
			if handlerCalls != 1 {
				t.Fatalf("handler calls = %d, want 1", handlerCalls)
			}
		})
	}
}

func TestRequestIDRequiresDependencies(t *testing.T) {
	t.Run("generator", func(t *testing.T) {
		assertPanic(t, func() {
			RequestID(nil)
		})
	})

	t.Run("next handler", func(t *testing.T) {
		assertPanic(t, func() {
			RequestID(func() uuid.UUID { return uuid.New() })(nil)
		})
	})

	t.Run("generator result", func(t *testing.T) {
		handler := RequestID(func() uuid.UUID { return uuid.Nil })(http.HandlerFunc(
			func(http.ResponseWriter, *http.Request) {},
		))

		assertPanic(t, func() {
			handler.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, "/", nil),
			)
		})
	})
}

func TestRequestIDFromContextReturnsFalseWhenMissing(t *testing.T) {
	if requestID, ok := RequestIDFromContext(context.Background()); ok || requestID != "" {
		t.Fatalf("RequestIDFromContext() = %q, %t; want empty, false", requestID, ok)
	}
}

func assertContextRequestID(t *testing.T, ctx context.Context, want string) {
	t.Helper()

	requestID, ok := RequestIDFromContext(ctx)
	if !ok {
		t.Fatal("request ID is missing from context")
	}
	if requestID != want {
		t.Fatalf("context request ID = %q, want %q", requestID, want)
	}
}
