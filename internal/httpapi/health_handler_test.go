package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLivenessReturnsOKWithoutCheckingReadiness(t *testing.T) {
	readiness := &fakeReadinessChecker{}
	handler := healthHandler{readiness: readiness, timeout: readinessTimeout}
	response := httptest.NewRecorder()

	handler.live(
		response,
		httptest.NewRequest(http.MethodGet, "/health/live", nil),
	)

	assertHealthOK(t, response)
	if readiness.calls != 0 {
		t.Fatalf("readiness calls = %d, want 0", readiness.calls)
	}
}

func TestReadinessReturnsOKAndUsesBoundedContext(t *testing.T) {
	readiness := &fakeReadinessChecker{
		ping: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("readiness context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > readinessTimeout {
				t.Fatalf("readiness deadline remaining = %s, want at most %s", remaining, readinessTimeout)
			}
			return nil
		},
	}
	handler := healthHandler{readiness: readiness, timeout: readinessTimeout}
	response := httptest.NewRecorder()

	handler.ready(
		response,
		httptest.NewRequest(http.MethodGet, "/health/ready", nil),
	)

	assertHealthOK(t, response)
	if readiness.calls != 1 {
		t.Fatalf("readiness calls = %d, want 1", readiness.calls)
	}
}

func TestReadinessReturnsGenericServiceUnavailable(t *testing.T) {
	const databaseError = "sensitive database detail"

	readiness := &fakeReadinessChecker{ping: func(context.Context) error {
		return errors.New(databaseError)
	}}
	handler := healthHandler{readiness: readiness, timeout: readinessTimeout}
	response := httptest.NewRecorder()

	handler.ready(
		response,
		httptest.NewRequest(http.MethodGet, "/health/ready", nil),
	)

	assertAPIError(
		t,
		response,
		http.StatusServiceUnavailable,
		errorCodeServiceUnavailable,
	)
	if strings.Contains(response.Body.String(), databaseError) {
		t.Fatal("readiness response exposes the dependency error")
	}
}

func TestReadinessTimeoutReturnsServiceUnavailable(t *testing.T) {
	const timeout = 10 * time.Millisecond

	readiness := &fakeReadinessChecker{ping: func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}}
	handler := healthHandler{readiness: readiness, timeout: timeout}
	response := httptest.NewRecorder()

	handler.ready(
		response,
		httptest.NewRequest(http.MethodGet, "/health/ready", nil),
	)

	assertAPIError(
		t,
		response,
		http.StatusServiceUnavailable,
		errorCodeServiceUnavailable,
	)
}

func TestReadinessPreservesParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	readiness := &fakeReadinessChecker{ping: func(ctx context.Context) error {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("readiness context error = %v, want context canceled", ctx.Err())
		}
		return ctx.Err()
	}}
	handler := healthHandler{readiness: readiness, timeout: readinessTimeout}
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil).WithContext(parent)
	underlying := httptest.NewRecorder()
	response := &statusResponseWriter{ResponseWriter: underlying}

	handler.ready(response, request)

	if readiness.calls != 1 {
		t.Fatalf("readiness calls = %d, want 1", readiness.calls)
	}
	if response.committed {
		t.Fatal("response was committed after request cancellation")
	}
	if underlying.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", underlying.Body.String())
	}
}

func TestHealthRoutesArePublic(t *testing.T) {
	authenticationCalls := 0
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			authenticationCalls++
			next.ServeHTTP(writer, request)
		})
	}
	readiness := &fakeReadinessChecker{}
	router := NewRouter(&fakeCompanyService{}, authenticate, readiness)

	for _, path := range []string{"/health/live", "/health/ready"} {
		response := performRequest(t, router, http.MethodGet, path, "", "")
		assertHealthOK(t, response)
	}

	if authenticationCalls != 0 {
		t.Fatalf("authentication calls = %d, want 0", authenticationCalls)
	}
	if readiness.calls != 1 {
		t.Fatalf("readiness calls = %d, want 1", readiness.calls)
	}
}

func TestNewRouterRequiresReadinessChecker(t *testing.T) {
	assertPanic(t, func() {
		NewRouter(&fakeCompanyService{}, passThroughAuthentication, nil)
	})
}

type fakeReadinessChecker struct {
	ping  func(context.Context) error
	calls int
}

func (checker *fakeReadinessChecker) Ping(ctx context.Context) error {
	checker.calls++
	if checker.ping == nil {
		return nil
	}

	return checker.ping(ctx)
}

func assertHealthOK(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("health status = %q, want ok", body.Status)
	}
}
