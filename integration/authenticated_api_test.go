//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/panagiotspappas/xm-company-service/internal/auth"
	"github.com/panagiotspappas/xm-company-service/internal/company"
	"github.com/panagiotspappas/xm-company-service/internal/httpapi"
	postgresrepository "github.com/panagiotspappas/xm-company-service/internal/postgres"
)

const (
	integrationJWTSecret      = "integration-test-secret-at-least-32-bytes"
	integrationJWTWrongSecret = "different-integration-test-secret-at-least-32-bytes"
	integrationJWTIssuer      = "xm-company-service-integration"
	integrationJWTAudience    = "xm-company-service-integration-client"
)

func TestAuthenticatedAPIRequiresJWT(t *testing.T) {
	api := newAuthenticatedIntegrationAPI(t)
	now := time.Now().UTC()

	missingAuthRequest := companyAPIRequest{
		Name:              integrationName("miss-"),
		Description:       "Missing authentication",
		AmountOfEmployees: 1,
		Registered:        true,
		Type:              company.TypeCorporations,
	}
	response := api.postCompany(t, missingAuthRequest, "")
	assertAuthenticationFailure(t, response)
	assertNoCompanyNamed(t, api.pool, missingAuthRequest.Name)

	tests := map[string]struct {
		namePrefix string
		token      string
	}{
		"wrong signature": {
			namePrefix: "sig-",
			token: signIntegrationToken(
				t,
				integrationJWTWrongSecret,
				now,
				now.Add(time.Hour),
			),
		},
		"expired token": {
			namePrefix: "exp-",
			token: signIntegrationToken(
				t,
				integrationJWTSecret,
				now.Add(-2*time.Hour),
				now.Add(-time.Hour),
			),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := companyAPIRequest{
				Name:              integrationName(test.namePrefix),
				Description:       "Rejected authentication",
				AmountOfEmployees: 1,
				Registered:        true,
				Type:              company.TypeCorporations,
			}

			response := api.postCompany(t, request, test.token)
			assertAuthenticationFailure(t, response)
			assertNoCompanyNamed(t, api.pool, request.Name)
		})
	}
}

func TestAuthenticatedAPIValidJWTReachesPostgreSQL(t *testing.T) {
	api := newAuthenticatedIntegrationAPI(t)
	now := time.Now().UTC()
	validToken := signIntegrationToken(
		t,
		integrationJWTSecret,
		now,
		now.Add(time.Hour),
	)
	request := companyAPIRequest{
		Name:              integrationName("auth-"),
		Description:       "Authenticated integration company",
		AmountOfEmployees: 7,
		Registered:        true,
		Type:              company.TypeCooperative,
	}

	createResponse := api.postCompany(t, request, validToken)
	if createResponse.StatusCode != http.StatusCreated {
		body := readAndClose(t, createResponse)
		t.Fatalf("POST status = %d, want %d; body = %s", createResponse.StatusCode, http.StatusCreated, body)
	}
	registerIntegrationCleanup(t, api.pool, request.Name)

	var created companyAPIResponse
	decodeAndClose(t, createResponse, &created)
	if got, want := createResponse.Header.Get("Location"), "/v1/companies/"+created.ID.String(); got != want {
		t.Fatalf("POST Location = %q, want %q", got, want)
	}
	want := companyAPIResponse{
		ID:                created.ID,
		Name:              request.Name,
		Description:       request.Description,
		AmountOfEmployees: request.AmountOfEmployees,
		Registered:        request.Registered,
		Type:              request.Type,
	}
	assertCompanyResponse(t, created, want)

	publicGetResponse := doJSONRequest(
		t,
		api.client,
		http.MethodGet,
		api.baseURL+"/v1/companies/"+created.ID.String(),
		"",
	)
	if publicGetResponse.StatusCode != http.StatusOK {
		body := readAndClose(t, publicGetResponse)
		t.Fatalf("public GET status = %d, want %d; body = %s", publicGetResponse.StatusCode, http.StatusOK, body)
	}
	var fetched companyAPIResponse
	decodeAndClose(t, publicGetResponse, &fetched)
	assertCompanyResponse(t, fetched, created)

	conflictResponse := api.postCompany(t, request, validToken)
	assertIntegrationAPIError(t, conflictResponse, http.StatusConflict, "COMPANY_NAME_CONFLICT")

	invalidRequest := request
	invalidRequest.Name = ""
	invalidResponse := api.postCompany(t, invalidRequest, validToken)
	assertIntegrationAPIError(t, invalidResponse, http.StatusBadRequest, "INVALID_REQUEST")

	finalGetResponse := doJSONRequest(
		t,
		api.client,
		http.MethodGet,
		api.baseURL+"/v1/companies/"+created.ID.String(),
		"",
	)
	if finalGetResponse.StatusCode != http.StatusOK {
		body := readAndClose(t, finalGetResponse)
		t.Fatalf("final GET status = %d, want %d; body = %s", finalGetResponse.StatusCode, http.StatusOK, body)
	}
	var persisted companyAPIResponse
	decodeAndClose(t, finalGetResponse, &persisted)
	assertCompanyResponse(t, persisted, created)
}

type authenticatedIntegrationAPI struct {
	pool    *pgxpool.Pool
	client  *http.Client
	baseURL string
}

func newAuthenticatedIntegrationAPI(t *testing.T) authenticatedIntegrationAPI {
	t.Helper()

	pool := openIntegrationPool(t)
	repository := postgresrepository.NewRepository(pool)
	service := company.NewService(repository, uuid.NewRandom)
	validator := auth.NewValidator(
		integrationJWTSecret,
		integrationJWTIssuer,
		integrationJWTAudience,
	)
	server := httptest.NewServer(httpapi.NewRouter(
		service,
		httpapi.RequireAuthentication(validator),
		pool,
	))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = integrationOperationTimeout

	return authenticatedIntegrationAPI{
		pool:    pool,
		client:  client,
		baseURL: server.URL,
	}
}

func (api authenticatedIntegrationAPI) postCompany(
	t *testing.T,
	request companyAPIRequest,
	rawToken string,
) *http.Response {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode authenticated POST request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		api.baseURL+"/v1/companies",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create authenticated POST request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if rawToken != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+rawToken)
	}

	response, err := api.client.Do(httpRequest)
	if err != nil {
		t.Fatalf("perform authenticated POST request: %v", err)
	}
	return response
}

func signIntegrationToken(
	t *testing.T,
	secret string,
	issuedAt time.Time,
	expiresAt time.Time,
) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Issuer:    integrationJWTIssuer,
		Audience:  jwt.ClaimStrings{integrationJWTAudience},
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	rawToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign integration JWT: %v", err)
	}
	return rawToken
}

func assertAuthenticationFailure(t *testing.T, response *http.Response) {
	t.Helper()

	if got := response.Header.Get("WWW-Authenticate"); got != "Bearer" {
		closeResponseBody(t, response)
		t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
	}
	assertIntegrationAPIError(t, response, http.StatusUnauthorized, "UNAUTHORIZED")
}

func assertNoCompanyNamed(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), integrationOperationTimeout)
	defer cancel()

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE name = $1`, name).Scan(&count); err != nil {
		t.Fatalf("count companies named %q: %v", name, err)
	}
	if count != 0 {
		t.Fatalf("companies named %q = %d, want 0", name, count)
	}
}
