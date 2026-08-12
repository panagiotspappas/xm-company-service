package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRequestLoggerCapturesExplicitStatuses(t *testing.T) {
	tests := []int{
		http.StatusNoContent,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, status := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var logs bytes.Buffer
			handlerCalls := 0
			handler := loggedHandler(&logs, http.HandlerFunc(
				func(writer http.ResponseWriter, _ *http.Request) {
					handlerCalls++
					writer.WriteHeader(status)
				},
			))
			request := httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != status {
				t.Fatalf("response status = %d, want %d", response.Code, status)
			}
			if handlerCalls != 1 {
				t.Fatalf("handler calls = %d, want 1", handlerCalls)
			}
			record := decodeSingleLogRecord(t, &logs)
			assertLogNumber(t, record, "status", status)
		})
	}
}

func TestRequestLoggerCapturesImplicitOK(t *testing.T) {
	tests := map[string]http.Handler{
		"body write": http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("ok"))
		}),
		"empty handler": http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	}

	for name, next := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := loggedHandler(&logs, next)
			response := httptest.NewRecorder()

			handler.ServeHTTP(
				response,
				httptest.NewRequest(http.MethodGet, "/health/live", nil),
			)

			record := decodeSingleLogRecord(t, &logs)
			assertLogNumber(t, record, "status", http.StatusOK)
		})
	}
}

func TestRequestLoggerCapturesFirstFinalStatus(t *testing.T) {
	var logs bytes.Buffer
	handler := loggedHandler(&logs, http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
			writer.WriteHeader(http.StatusInternalServerError)
		},
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodDelete, "/v1/companies/id", nil),
	)

	if response.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", response.Code, http.StatusNoContent)
	}
	record := decodeSingleLogRecord(t, &logs)
	assertLogNumber(t, record, "status", http.StatusNoContent)
}

func TestRequestLoggerEmitsSafeCompletionRecord(t *testing.T) {
	const (
		querySecret = "sensitive-query-value"
		tokenSecret = "sensitive-token-value"
	)

	var logs bytes.Buffer
	handler := loggedHandler(&logs, http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(
		http.MethodPatch,
		"/v1/companies/id?secret="+querySecret,
		nil,
	)
	request.Header.Set("Authorization", "Bearer "+tokenSecret)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	record := decodeSingleLogRecord(t, &logs)
	if got := record["level"]; got != slog.LevelInfo.String() {
		t.Fatalf("level = %v, want %s", got, slog.LevelInfo)
	}
	if got := record["msg"]; got != "http request completed" {
		t.Fatalf("message = %v, want http request completed", got)
	}
	if got := record["request_id"]; got != generatedRequestID {
		t.Fatalf("request_id = %v, want %s", got, generatedRequestID)
	}
	if got := record["method"]; got != http.MethodPatch {
		t.Fatalf("method = %v, want %s", got, http.MethodPatch)
	}
	if got := record["path"]; got != "/v1/companies/id" {
		t.Fatalf("path = %v, want /v1/companies/id", got)
	}
	assertLogNumber(t, record, "status", http.StatusNoContent)
	if _, ok := record["duration"]; !ok {
		t.Fatal("duration is missing from completion record")
	}
	if strings.Contains(logs.String(), querySecret) {
		t.Fatal("completion record contains the query string")
	}
	if strings.Contains(logs.String(), tokenSecret) {
		t.Fatal("completion record contains the authorization token")
	}
}

func TestRequestLoggerCapturesAuthenticationFailure(t *testing.T) {
	var logs bytes.Buffer
	handlerCalls := 0
	authenticated := RequireAuthentication(&fakeTokenValidator{})(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			handlerCalls++
		},
	))
	handler := loggedHandler(&logs, authenticated)
	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/v1/companies", nil),
	)

	assertUnauthorized(t, response)
	if handlerCalls != 0 {
		t.Fatalf("handler calls = %d, want 0", handlerCalls)
	}
	record := decodeSingleLogRecord(t, &logs)
	assertLogNumber(t, record, "status", http.StatusUnauthorized)
}

func TestStatusResponseWriterUnwrapsUnderlyingWriter(t *testing.T) {
	underlying := httptest.NewRecorder()
	writer := &statusResponseWriter{ResponseWriter: underlying}

	if got := writer.Unwrap(); got != underlying {
		t.Fatalf("Unwrap() = %T, want underlying response writer", got)
	}
}

func TestRequestLoggerRequiresDependencies(t *testing.T) {
	t.Run("logger", func(t *testing.T) {
		assertPanic(t, func() {
			RequestLogger(nil)
		})
	})

	t.Run("next handler", func(t *testing.T) {
		assertPanic(t, func() {
			RequestLogger(slog.Default())(nil)
		})
	})
}

func loggedHandler(logs *bytes.Buffer, next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return RequestID(func() uuid.UUID {
		return uuid.MustParse(generatedRequestID)
	})(RequestLogger(logger)(next))
}

func decodeSingleLogRecord(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()

	decoder := json.NewDecoder(logs)
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		t.Fatalf("decode completion record: %v", err)
	}

	var additional map[string]any
	if err := decoder.Decode(&additional); err != io.EOF {
		t.Fatalf("additional completion record: %v", err)
	}

	return record
}

func assertLogNumber(t *testing.T, record map[string]any, key string, want int) {
	t.Helper()

	got, ok := record[key].(float64)
	if !ok {
		t.Fatalf("%s = %T(%v), want number", key, record[key], record[key])
	}
	if int(got) != want {
		t.Fatalf("%s = %v, want %d", key, got, want)
	}
}
