package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestRecoverPanicsReturnsGenericInternalError(t *testing.T) {
	const panicValue = "sensitive panic detail"

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := phase63Handler(logger, time.Minute, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			panic(panicValue)
		},
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil),
	)

	assertAPIError(t, response, http.StatusInternalServerError, errorCodeInternal)
	if strings.Contains(response.Body.String(), panicValue) {
		t.Fatal("panic value was exposed to the client")
	}
	if strings.Contains(response.Body.String(), "goroutine") {
		t.Fatal("panic stack was exposed to the client")
	}

	records := decodeLogRecords(t, &logs)
	panicRecord := findLogRecord(t, records, "http request panic")
	if got := panicRecord["level"]; got != slog.LevelError.String() {
		t.Fatalf("panic log level = %v, want %s", got, slog.LevelError)
	}
	if got := panicRecord["request_id"]; got != generatedRequestID {
		t.Fatalf("panic request_id = %v, want %s", got, generatedRequestID)
	}
	if got := panicRecord["panic"]; got != panicValue {
		t.Fatalf("panic diagnostic = %v, want %s", got, panicValue)
	}
	stack, ok := panicRecord["stack"].(string)
	if !ok || !strings.Contains(stack, "goroutine") {
		t.Fatalf("panic stack = %T(%v), want Go stack trace", panicRecord["stack"], panicRecord["stack"])
	}

	completionRecord := findLogRecord(t, records, "http request completed")
	assertLogNumber(t, completionRecord, "status", http.StatusInternalServerError)
}

func TestRecoverPanicsDoesNotAppendErrorAfterCommit(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := RecoverPanics(logger)(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte("partial response"))
			panic("post-commit panic")
		},
	))
	response := httptest.NewRecorder()

	func() {
		defer func() {
			recovered := recover()
			if recovered != http.ErrAbortHandler {
				t.Fatalf("recovered panic = %v, want http.ErrAbortHandler", recovered)
			}
		}()

		handler.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil),
		)
	}()

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
	if got := response.Body.String(); got != "partial response" {
		t.Fatalf("body = %q, want only the committed response", got)
	}
	findLogRecord(t, decodeLogRecords(t, &logs), "http request panic")
}

func TestRecoverPanicsKeepsHandlerAvailable(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	calls := 0
	handler := RecoverPanics(logger)(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				panic("first request")
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/first", nil))
	assertAPIError(t, first, http.StatusInternalServerError, errorCodeInternal)

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/second", nil))
	if second.Code != http.StatusNoContent {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusNoContent)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
}

func TestRecoverPanicsRequiresDependencies(t *testing.T) {
	t.Run("logger", func(t *testing.T) {
		assertPanic(t, func() {
			RecoverPanics(nil)
		})
	})

	t.Run("next handler", func(t *testing.T) {
		assertPanic(t, func() {
			RecoverPanics(slog.Default())(nil)
		})
	})
}

func TestRequestTimeoutInstallsDeadline(t *testing.T) {
	const timeout = time.Minute

	started := time.Now()
	handlerCalls := 0
	handler := RequestTimeout(timeout)(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			handlerCalls++
			deadline, ok := request.Context().Deadline()
			if !ok {
				t.Fatal("request context has no deadline")
			}
			gotTimeout := deadline.Sub(started)
			if gotTimeout <= timeout-time.Second || gotTimeout > timeout+time.Second {
				t.Fatalf("request timeout = %s, want approximately %s", gotTimeout, timeout)
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil),
	)

	if handlerCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handlerCalls)
	}
}

func TestRequestTimeoutCancelsBlockingDownstreamWork(t *testing.T) {
	const timeout = 10 * time.Millisecond

	var downstreamErr error
	handler := RequestTimeout(timeout)(http.HandlerFunc(
		func(_ http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
			downstreamErr = request.Context().Err()
		},
	))

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil),
	)

	if !errors.Is(downstreamErr, context.DeadlineExceeded) {
		t.Fatalf("downstream error = %v, want context deadline exceeded", downstreamErr)
	}
}

func TestRequestTimeoutPreservesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	var downstreamErr error
	handler := RequestTimeout(time.Minute)(http.HandlerFunc(
		func(_ http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
			downstreamErr = request.Context().Err()
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil)
	request = request.WithContext(parent)

	handler.ServeHTTP(httptest.NewRecorder(), request)

	if !errors.Is(downstreamErr, context.Canceled) {
		t.Fatalf("downstream error = %v, want context canceled", downstreamErr)
	}
}

func TestRequestTimeoutRequiresDependencies(t *testing.T) {
	for name, timeout := range map[string]time.Duration{
		"zero":     0,
		"negative": -time.Second,
	} {
		t.Run(name, func(t *testing.T) {
			assertPanic(t, func() {
				RequestTimeout(timeout)
			})
		})
	}

	t.Run("next handler", func(t *testing.T) {
		assertPanic(t, func() {
			RequestTimeout(time.Second)(nil)
		})
	})
}

func TestRequestLoggerClassifiesUncommittedCancellationAs499(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	parent, cancel := context.WithCancel(context.Background())
	handler := RequestID(func() uuid.UUID {
		return uuid.MustParse(generatedRequestID)
	})(RequestLogger(logger)(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			cancel()
		},
	)))
	request := httptest.NewRequest(http.MethodGet, "/v1/companies/id", nil)
	request = request.WithContext(parent)

	handler.ServeHTTP(httptest.NewRecorder(), request)

	record := decodeSingleLogRecord(t, &logs)
	assertLogNumber(t, record, "status", clientClosedRequestStatus)
}

func loggedHandler(logs *bytes.Buffer, next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return RequestID(func() uuid.UUID {
		return uuid.MustParse(generatedRequestID)
	})(RequestLogger(logger)(next))
}

func phase63Handler(logger *slog.Logger, timeout time.Duration, next http.Handler) http.Handler {
	return RequestID(func() uuid.UUID {
		return uuid.MustParse(generatedRequestID)
	})(RequestLogger(logger)(RecoverPanics(logger)(RequestTimeout(timeout)(next))))
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

func decodeLogRecords(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatalf("decode log record: %v", err)
		}
		records = append(records, record)
	}
}

func findLogRecord(t *testing.T, records []map[string]any, message string) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}

	t.Fatalf("log record %q not found in %#v", message, records)
	return nil
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
