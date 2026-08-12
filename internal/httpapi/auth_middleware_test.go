package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRequireAuthenticationAcceptsBearerToken(t *testing.T) {
	tests := map[string]string{
		"canonical scheme":        "Bearer valid-token",
		"case-insensitive scheme": "bEaReR valid-token",
		"tab separator":           "Bearer\tvalid-token",
	}

	for name, authorization := range tests {
		t.Run(name, func(t *testing.T) {
			validator := &fakeTokenValidator{}
			handlerCalls := 0
			handler := RequireAuthentication(validator)(http.HandlerFunc(
				func(writer http.ResponseWriter, _ *http.Request) {
					handlerCalls++
					writer.WriteHeader(http.StatusNoContent)
				},
			))
			request := httptest.NewRequest(http.MethodPost, "/v1/companies", nil)
			request.Header.Set("Authorization", authorization)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body)
			}
			if handlerCalls != 1 {
				t.Fatalf("handler calls = %d, want 1", handlerCalls)
			}
			if len(validator.tokens) != 1 || validator.tokens[0] != "valid-token" {
				t.Fatalf("validated tokens = %#v, want [valid-token]", validator.tokens)
			}
		})
	}
}

func TestRequireAuthenticationRejectsMalformedAuthorization(t *testing.T) {
	tests := map[string][]string{
		"missing":          nil,
		"empty":            {""},
		"scheme only":      {"Bearer"},
		"token only":       {"valid-token"},
		"wrong scheme":     {"Basic valid-token"},
		"empty token":      {"Bearer   "},
		"extra field":      {"Bearer valid-token extra"},
		"duplicate header": {"Bearer valid-token", "Bearer another-token"},
	}

	for name, authorizationValues := range tests {
		t.Run(name, func(t *testing.T) {
			validator := &fakeTokenValidator{}
			handlerCalls := 0
			handler := RequireAuthentication(validator)(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) {
					handlerCalls++
				},
			))
			request := httptest.NewRequest(http.MethodPost, "/v1/companies", nil)
			for _, value := range authorizationValues {
				request.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			assertUnauthorized(t, response)
			if handlerCalls != 0 {
				t.Fatalf("handler calls = %d, want 0", handlerCalls)
			}
			if len(validator.tokens) != 0 {
				t.Fatalf("Validate() tokens = %#v, want no calls", validator.tokens)
			}
		})
	}
}

func TestRequireAuthenticationRejectsInvalidToken(t *testing.T) {
	validator := &fakeTokenValidator{err: errors.New("sensitive verification detail")}
	handlerCalls := 0
	handler := RequireAuthentication(validator)(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			handlerCalls++
		},
	))
	request := httptest.NewRequest(http.MethodPost, "/v1/companies", nil)
	request.Header.Set("Authorization", "Bearer invalid-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertUnauthorized(t, response)
	if handlerCalls != 0 {
		t.Fatalf("handler calls = %d, want 0", handlerCalls)
	}
	if len(validator.tokens) != 1 || validator.tokens[0] != "invalid-token" {
		t.Fatalf("validated tokens = %#v, want [invalid-token]", validator.tokens)
	}
	if strings.Contains(response.Body.String(), "sensitive") {
		t.Fatalf("response leaks validation error: %s", response.Body)
	}
}

func TestRequireAuthenticationRequiresDependencies(t *testing.T) {
	t.Run("validator", func(t *testing.T) {
		assertPanic(t, func() {
			RequireAuthentication(nil)
		})
	})

	t.Run("next handler", func(t *testing.T) {
		assertPanic(t, func() {
			RequireAuthentication(&fakeTokenValidator{})(nil)
		})
	})
}

func TestRouterAuthenticationBoundary(t *testing.T) {
	authenticationCalls := map[string]int{}
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			authenticationCalls[request.Method]++
			next.ServeHTTP(writer, request)
		})
	}
	router := NewRouter(&fakeCompanyService{}, authenticate, alwaysReadyChecker{})
	id := uuid.NewString()
	tests := []struct {
		method      string
		path        string
		body        string
		contentType string
		wantCalls   int
	}{
		{method: http.MethodGet, path: "/v1/companies/" + id, wantCalls: 0},
		{
			method:      http.MethodPost,
			path:        "/v1/companies",
			body:        `{"name":"Acme","amount_of_employees":1,"registered":true,"type":"Corporations"}`,
			contentType: "application/json",
			wantCalls:   1,
		},
		{
			method:      http.MethodPatch,
			path:        "/v1/companies/" + id,
			body:        `{"description":"Updated"}`,
			contentType: "application/json",
			wantCalls:   1,
		},
		{method: http.MethodDelete, path: "/v1/companies/" + id, wantCalls: 1},
	}

	for _, test := range tests {
		response := performRequest(
			t,
			router,
			test.method,
			test.path,
			test.body,
			test.contentType,
		)
		if response.Code >= http.StatusBadRequest {
			t.Fatalf("%s status = %d; body = %s", test.method, response.Code, response.Body)
		}
		if got := authenticationCalls[test.method]; got != test.wantCalls {
			t.Fatalf("%s authentication calls = %d, want %d", test.method, got, test.wantCalls)
		}
	}
}

func TestNewRouterRequiresAuthenticationMiddleware(t *testing.T) {
	assertPanic(t, func() {
		NewRouter(&fakeCompanyService{}, nil, alwaysReadyChecker{})
	})
}

func TestProtectedRoutesAuthenticateBeforeParsingRequests(t *testing.T) {
	router := NewRouter(
		&fakeCompanyService{},
		RequireAuthentication(&fakeTokenValidator{}),
		alwaysReadyChecker{},
	)
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/v1/companies", body: "not-json"},
		{method: http.MethodPatch, path: "/v1/companies/not-a-uuid", body: "not-json"},
		{method: http.MethodDelete, path: "/v1/companies/not-a-uuid"},
	}

	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			response := performRequest(
				t,
				router,
				test.method,
				test.path,
				test.body,
				"application/json",
			)

			assertUnauthorized(t, response)
		})
	}
}

type fakeTokenValidator struct {
	tokens []string
	err    error
}

func (validator *fakeTokenValidator) Validate(token string) error {
	validator.tokens = append(validator.tokens, token)
	return validator.err
}

func assertUnauthorized(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	assertAPIError(t, response, http.StatusUnauthorized, errorCodeUnauthorized)
	if got := response.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
	}

	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Message != "unauthorized" {
		t.Fatalf("error message = %q, want unauthorized", body.Error.Message)
	}
}

func assertPanic(t *testing.T, function func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("function did not panic")
		}
	}()

	function()
}
