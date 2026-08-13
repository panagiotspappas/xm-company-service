package main

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/panagiotspappas/xm-company-service/internal/company"
	"github.com/panagiotspappas/xm-company-service/internal/httpapi"
)

func TestNewHTTPServerConfiguration(t *testing.T) {
	server := newHTTPServer(
		"127.0.0.1:9090",
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)

	if server.Addr != "127.0.0.1:9090" {
		t.Fatalf("Addr = %q, want 127.0.0.1:9090", server.Addr)
	}
	if server.ReadHeaderTimeout != readHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", server.ReadHeaderTimeout, readHeaderTimeout)
	}
	if server.ReadTimeout != readTimeout {
		t.Fatalf("ReadTimeout = %s, want %s", server.ReadTimeout, readTimeout)
	}
	if server.WriteTimeout != writeTimeout {
		t.Fatalf("WriteTimeout = %s, want %s", server.WriteTimeout, writeTimeout)
	}
	if server.IdleTimeout != idleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", server.IdleTimeout, idleTimeout)
	}
}

func TestProductionMiddlewareAddsRequestIDAndCompletionLog(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
		method     string
		path       string
		wantStatus int
	}{
		{
			name: "success",
			handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			}),
			method:     http.MethodGet,
			path:       "/health/live",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "unauthorized",
			handler: httpapi.RequireAuthentication(&serverTokenValidator{})(http.HandlerFunc(
				func(writer http.ResponseWriter, _ *http.Request) {
					writer.WriteHeader(http.StatusNoContent)
				},
			)),
			method:     http.MethodPost,
			path:       "/v1/companies",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "native not found",
			handler:    http.NewServeMux(),
			method:     http.MethodGet,
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "recovered panic",
			handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic("test panic")
			}),
			method:     http.MethodGet,
			path:       "/panic",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			server := newHTTPServer(":0", test.handler, logger)
			response := httptest.NewRecorder()

			server.Handler.ServeHTTP(
				response,
				httptest.NewRequest(test.method, test.path, nil),
			)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body)
			}
			requestID := response.Header().Get("X-Request-ID")
			parsedRequestID, err := uuid.Parse(requestID)
			if err != nil || parsedRequestID == uuid.Nil {
				t.Fatalf("X-Request-ID = %q, want non-nil UUID", requestID)
			}

			record := findServerLogRecord(t, decodeServerLogRecords(t, &logs), "http request completed")
			if got := record["request_id"]; got != requestID {
				t.Fatalf("logged request_id = %v, want %s", got, requestID)
			}
			assertServerLogNumber(t, record, "status", test.wantStatus)
		})
	}
}

func TestProductionHandlerPreservesAuthenticationAndRequestDeadline(t *testing.T) {
	companyID := uuid.New()
	var getDeadline time.Time
	service := &serverCompanyService{
		get: func(ctx context.Context, _ uuid.UUID) (company.Company, error) {
			var ok bool
			getDeadline, ok = ctx.Deadline()
			if !ok {
				t.Fatal("GET context has no application deadline")
			}
			return serverTestCompany(companyID), nil
		},
		create: func(context.Context, company.CreateCompanyInput) (company.Company, error) {
			return serverTestCompany(companyID), nil
		},
	}
	validator := &serverTokenValidator{}
	router := httpapi.NewRouter(
		service,
		httpapi.RequireAuthentication(validator),
		serverReadinessChecker{},
	)
	server := newHTTPServer(
		":0",
		router,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)

	getResponse := httptest.NewRecorder()
	beforeServe := time.Now()
	server.Handler.ServeHTTP(
		getResponse,
		httptest.NewRequest(http.MethodGet, "/v1/companies/"+companyID.String(), nil),
	)
	afterServe := time.Now()
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body = %s", getResponse.Code, http.StatusOK, getResponse.Body)
	}
	if len(validator.tokens) != 0 {
		t.Fatalf("GET validated tokens = %#v, want none", validator.tokens)
	}
	earliestDeadline := beforeServe.Add(applicationRequestTimeout)
	latestDeadline := afterServe.Add(applicationRequestTimeout)
	if getDeadline.Before(earliestDeadline) || getDeadline.After(latestDeadline) {
		t.Fatalf(
			"application deadline = %s, want between %s and %s",
			getDeadline,
			earliestDeadline,
			latestDeadline,
		)
	}

	requestBody := `{"name":"Acme","amount_of_employees":1,"registered":true,"type":"Corporations"}`
	protectedRequests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/v1/companies", body: requestBody},
		{
			method: http.MethodPatch,
			path:   "/v1/companies/" + companyID.String(),
			body:   `{"description":"Updated"}`,
		},
		{method: http.MethodDelete, path: "/v1/companies/" + companyID.String()},
	}
	for _, protectedRequest := range protectedRequests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(
			protectedRequest.method,
			protectedRequest.path,
			strings.NewReader(protectedRequest.body),
		)
		if protectedRequest.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}

		server.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf(
				"unauthorized %s status = %d, want %d",
				protectedRequest.method,
				response.Code,
				http.StatusUnauthorized,
			)
		}
	}

	authorizedResponse := httptest.NewRecorder()
	authorizedRequest := httptest.NewRequest(
		http.MethodPost,
		"/v1/companies",
		strings.NewReader(requestBody),
	)
	authorizedRequest.Header.Set("Content-Type", "application/json")
	authorizedRequest.Header.Set("Authorization", "Bearer valid-token")
	server.Handler.ServeHTTP(authorizedResponse, authorizedRequest)
	if authorizedResponse.Code != http.StatusCreated {
		t.Fatalf("authorized POST status = %d, want %d; body = %s", authorizedResponse.Code, http.StatusCreated, authorizedResponse.Body)
	}
	if len(validator.tokens) != 1 || validator.tokens[0] != "valid-token" {
		t.Fatalf("validated tokens = %#v, want [valid-token]", validator.tokens)
	}
}

func TestServeHTTPHandlesImmediateServerResult(t *testing.T) {
	serverFailure := errors.New("listener failed")
	tests := []struct {
		name    string
		result  error
		wantErr error
	}{
		{name: "server closed", result: http.ErrServerClosed},
		{name: "nil result", result: nil},
		{name: "listener failure", result: serverFailure, wantErr: serverFailure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newFakeHTTPServer()
			server.listenResult <- test.result
			stopCalls := 0

			err := serveHTTP(
				server,
				context.Background(),
				func() { stopCalls++ },
				slog.New(slog.NewJSONHandler(io.Discard, nil)),
				time.Second,
			)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("serveHTTP() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil && !strings.Contains(err.Error(), "serve HTTP") {
				t.Fatalf("serveHTTP() error = %q, want server context", err)
			}
			if stopCalls != 0 {
				t.Fatalf("stop signal calls = %d, want 0", stopCalls)
			}
		})
	}
}

func TestServeHTTPRestoresSignalsBeforeGracefulShutdown(t *testing.T) {
	server := newFakeHTTPServer()
	var signalsStopped atomic.Bool
	var stoppedBeforeShutdown atomic.Bool
	var closeCalls atomic.Int32
	server.shutdown = func(context.Context) error {
		stoppedBeforeShutdown.Store(signalsStopped.Load())
		server.listenResult <- http.ErrServerClosed
		return nil
	}
	server.close = func() error {
		closeCalls.Add(1)
		return nil
	}
	shutdownContext, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()

	err := serveHTTP(
		server,
		shutdownContext,
		func() { signalsStopped.Store(true) },
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		time.Second,
	)

	if err != nil {
		t.Fatalf("serveHTTP() error = %v, want nil", err)
	}
	if !stoppedBeforeShutdown.Load() {
		t.Fatal("signal defaults were not restored before Shutdown")
	}
	if closeCalls.Load() != 0 {
		t.Fatalf("Close() calls = %d, want 0", closeCalls.Load())
	}
}

func TestServeHTTPWaitsAfterManagedHTTPClosure(t *testing.T) {
	tests := []struct {
		name           string
		shutdownResult error
	}{
		{name: "graceful shutdown"},
		{name: "forced closure", shutdownResult: errors.New("shutdown failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newFakeHTTPServer()
			closureCompleted := make(chan struct{})
			server.shutdown = func(context.Context) error {
				if test.shutdownResult == nil {
					close(closureCompleted)
				}
				return test.shutdownResult
			}
			server.close = func() error {
				close(closureCompleted)
				return nil
			}
			shutdownContext, cancelShutdown := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				result <- serveHTTP(
					server,
					shutdownContext,
					func() {},
					slog.New(slog.NewJSONHandler(io.Discard, nil)),
					time.Second,
				)
			}()

			<-server.listenStarted
			cancelShutdown()
			<-closureCompleted
			select {
			case err := <-result:
				t.Fatalf("serveHTTP() returned before ListenAndServe exited: %v", err)
			case <-time.After(20 * time.Millisecond):
			}

			server.listenResult <- http.ErrServerClosed
			if err := waitForServeResult(t, result); err != nil {
				t.Fatalf("serveHTTP() error = %v, want nil", err)
			}
		})
	}
}

func TestServeHTTPLogsShutdownFailureAndForcesClose(t *testing.T) {
	server := newFakeHTTPServer()
	server.shutdown = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	server.close = func() error {
		server.listenResult <- http.ErrServerClosed
		return nil
	}
	shutdownContext, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	var logs bytes.Buffer

	err := serveHTTP(
		server,
		shutdownContext,
		func() {},
		slog.New(slog.NewJSONHandler(&logs, nil)),
		10*time.Millisecond,
	)

	if err != nil {
		t.Fatalf("serveHTTP() error = %v, want nil", err)
	}
	record := findServerLogRecord(
		t,
		decodeServerLogRecords(t, &logs),
		"HTTP server graceful shutdown failed",
	)
	if got := record["level"]; got != slog.LevelError.String() {
		t.Fatalf("shutdown log level = %v, want %s", got, slog.LevelError)
	}
}

func TestServeHTTPReturnsForceCloseFailureWithoutWaiting(t *testing.T) {
	shutdownFailure := errors.New("shutdown failed")
	closeFailure := errors.New("close failed")
	server := newFakeHTTPServer()
	server.shutdown = func(context.Context) error { return shutdownFailure }
	server.close = func() error { return closeFailure }
	shutdownContext, cancelShutdown := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- serveHTTP(
			server,
			shutdownContext,
			func() {},
			slog.New(slog.NewJSONHandler(io.Discard, nil)),
			time.Second,
		)
	}()

	<-server.listenStarted
	cancelShutdown()
	err := waitForServeResult(t, result)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("serveHTTP() error = %v, want force-close failure", err)
	}
	if !strings.Contains(err.Error(), "force close HTTP server") {
		t.Fatalf("serveHTTP() error = %q, want force-close context", err)
	}
	server.listenResult <- http.ErrServerClosed
}

func TestServeHTTPPropagatesServerFailureAfterShutdown(t *testing.T) {
	listenerFailure := errors.New("listener failed during shutdown")
	tests := []struct {
		name           string
		shutdownResult error
	}{
		{name: "graceful shutdown"},
		{name: "forced closure", shutdownResult: errors.New("shutdown failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newFakeHTTPServer()
			server.shutdown = func(context.Context) error {
				if test.shutdownResult == nil {
					server.listenResult <- listenerFailure
				}
				return test.shutdownResult
			}
			server.close = func() error {
				server.listenResult <- listenerFailure
				return nil
			}
			shutdownContext, cancelShutdown := context.WithCancel(context.Background())
			cancelShutdown()

			err := serveHTTP(
				server,
				shutdownContext,
				func() {},
				slog.New(slog.NewJSONHandler(io.Discard, nil)),
				time.Second,
			)

			if !errors.Is(err, listenerFailure) {
				t.Fatalf("serveHTTP() error = %v, want listener failure", err)
			}
		})
	}
}

type fakeHTTPServer struct {
	listenStarted chan struct{}
	listenResult  chan error
	shutdown      func(context.Context) error
	close         func() error
}

func newFakeHTTPServer() *fakeHTTPServer {
	return &fakeHTTPServer{
		listenStarted: make(chan struct{}),
		listenResult:  make(chan error, 1),
		shutdown:      func(context.Context) error { return nil },
		close:         func() error { return nil },
	}
}

func (server *fakeHTTPServer) ListenAndServe() error {
	close(server.listenStarted)
	return <-server.listenResult
}

func (server *fakeHTTPServer) Shutdown(ctx context.Context) error {
	return server.shutdown(ctx)
}

func (server *fakeHTTPServer) Close() error {
	return server.close()
}

type serverCompanyService struct {
	create func(context.Context, company.CreateCompanyInput) (company.Company, error)
	get    func(context.Context, uuid.UUID) (company.Company, error)
}

func (service *serverCompanyService) Create(
	ctx context.Context,
	input company.CreateCompanyInput,
) (company.Company, error) {
	return service.create(ctx, input)
}

func (service *serverCompanyService) Get(
	ctx context.Context,
	id uuid.UUID,
) (company.Company, error) {
	return service.get(ctx, id)
}

func (*serverCompanyService) Patch(
	context.Context,
	uuid.UUID,
	company.PatchCompanyInput,
) (company.Company, error) {
	return company.Company{}, nil
}

func (*serverCompanyService) Delete(context.Context, uuid.UUID) error {
	return nil
}

type serverTokenValidator struct {
	tokens []string
}

func (validator *serverTokenValidator) Validate(token string) error {
	validator.tokens = append(validator.tokens, token)
	return nil
}

type serverReadinessChecker struct{}

func (serverReadinessChecker) Ping(context.Context) error {
	return nil
}

func serverTestCompany(id uuid.UUID) company.Company {
	return company.Company{
		ID:                id,
		Name:              "Acme",
		Description:       "Test company",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeCorporations,
	}
}

func waitForServeResult(t *testing.T, result <-chan error) error {
	t.Helper()

	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("serveHTTP did not return")
		return nil
	}
}

func decodeServerLogRecords(t *testing.T, logs *bytes.Buffer) []map[string]any {
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

func findServerLogRecord(
	t *testing.T,
	records []map[string]any,
	message string,
) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}

	t.Fatalf("log record %q not found in %#v", message, records)
	return nil
}

func assertServerLogNumber(t *testing.T, record map[string]any, key string, want int) {
	t.Helper()

	got, ok := record[key].(float64)
	if !ok {
		t.Fatalf("%s = %T(%v), want number", key, record[key], record[key])
	}
	if int(got) != want {
		t.Fatalf("%s = %v, want %d", key, got, want)
	}
}
