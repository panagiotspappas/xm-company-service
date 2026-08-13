//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/panagiotspappas/xm-company-service/internal/company"
	"github.com/panagiotspappas/xm-company-service/internal/httpapi"
	postgresrepository "github.com/panagiotspappas/xm-company-service/internal/postgres"
)

const integrationOperationTimeout = 5 * time.Second

func TestCompanyPostAndGetVerticalSlice(t *testing.T) {
	pool := openIntegrationPool(t)
	repository := postgresrepository.NewRepository(pool)
	service := company.NewService(repository, uuid.NewRandom)
	server := httptest.NewServer(httpapi.NewRouter(service, passThroughAuthentication, pool))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = integrationOperationTimeout

	name := "api-" + uuid.NewString()[:8]
	createBody := fmt.Sprintf(`{
		"name": %q,
		"description": "Integration company",
		"amount_of_employees": 0,
		"registered": false,
		"type": "Corporations"
	}`, name)

	createResponse := doJSONRequest(t, client, http.MethodPost, server.URL+"/v1/companies", createBody)
	if createResponse.StatusCode != http.StatusCreated {
		body := readAndClose(t, createResponse)
		t.Fatalf("POST status = %d, want %d; body = %s", createResponse.StatusCode, http.StatusCreated, body)
	}
	registerIntegrationCleanup(t, pool, name)
	var created companyAPIResponse
	decodeAndClose(t, createResponse, &created)
	if created.Name != name || created.Description != "Integration company" || created.AmountOfEmployees != 0 || created.Registered {
		t.Fatalf("POST company = %#v, want submitted company", created)
	}
	if got, want := createResponse.Header.Get("Location"), "/v1/companies/"+created.ID.String(); got != want {
		t.Fatalf("POST Location = %q, want %q", got, want)
	}

	getResponse := doJSONRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/v1/companies/"+created.ID.String(),
		"",
	)
	if getResponse.StatusCode != http.StatusOK {
		body := readAndClose(t, getResponse)
		t.Fatalf("GET status = %d, want %d; body = %s", getResponse.StatusCode, http.StatusOK, body)
	}
	var fetched companyAPIResponse
	decodeAndClose(t, getResponse, &fetched)
	if fetched != created {
		t.Fatalf("GET company = %#v, want %#v", fetched, created)
	}

	duplicateResponse := doJSONRequest(t, client, http.MethodPost, server.URL+"/v1/companies", createBody)
	defer closeResponseBody(t, duplicateResponse)
	if duplicateResponse.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(duplicateResponse.Body)
		t.Fatalf("duplicate POST status = %d, want %d; body = %s", duplicateResponse.StatusCode, http.StatusConflict, body)
	}

	missingResponse := doJSONRequest(
		t,
		client,
		http.MethodGet,
		server.URL+"/v1/companies/"+uuid.NewString(),
		"",
	)
	defer closeResponseBody(t, missingResponse)
	if missingResponse.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(missingResponse.Body)
		t.Fatalf("missing GET status = %d, want %d; body = %s", missingResponse.StatusCode, http.StatusNotFound, body)
	}
}

func TestPatchCompanyPersistsPartialUpdate(t *testing.T) {
	api := newIntegrationAPI(t)
	original := api.createCompany(t, companyAPIRequest{
		Name:              integrationName("part-"),
		Description:       "Original description",
		AmountOfEmployees: 25,
		Registered:        true,
		Type:              company.TypeCorporations,
	})

	updated := api.patchCompany(t, original.ID, `{"description":"Updated description"}`)
	want := original
	want.Description = "Updated description"
	assertCompanyResponse(t, updated, want)

	persisted := api.getCompany(t, original.ID)
	assertCompanyResponse(t, persisted, want)
}

func TestPatchCompanyPersistsExplicitZeroValues(t *testing.T) {
	tests := map[string]struct {
		namePrefix string
		patchBody  string
		updateWant func(*companyAPIResponse)
	}{
		"employee count zero": {
			namePrefix: "emp-",
			patchBody:  `{"amount_of_employees":0}`,
			updateWant: func(value *companyAPIResponse) { value.AmountOfEmployees = 0 },
		},
		"registered false": {
			namePrefix: "reg-",
			patchBody:  `{"registered":false}`,
			updateWant: func(value *companyAPIResponse) { value.Registered = false },
		},
		"clear description": {
			namePrefix: "desc-",
			patchBody:  `{"description":""}`,
			updateWant: func(value *companyAPIResponse) { value.Description = "" },
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			api := newIntegrationAPI(t)
			original := api.createCompany(t, companyAPIRequest{
				Name:              integrationName(test.namePrefix),
				Description:       "Original description",
				AmountOfEmployees: 25,
				Registered:        true,
				Type:              company.TypeCorporations,
			})

			want := original
			test.updateWant(&want)
			updated := api.patchCompany(t, original.ID, test.patchBody)
			assertCompanyResponse(t, updated, want)

			persisted := api.getCompany(t, original.ID)
			assertCompanyResponse(t, persisted, want)
		})
	}
}

func TestPatchCompanyNameConflictPreservesState(t *testing.T) {
	api := newIntegrationAPI(t)
	first := api.createCompany(t, companyAPIRequest{
		Name:              integrationName("con-a-"),
		Description:       "First company",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeCorporations,
	})
	second := api.createCompany(t, companyAPIRequest{
		Name:              integrationName("con-b-"),
		Description:       "Second company",
		AmountOfEmployees: 20,
		Registered:        false,
		Type:              company.TypeCooperative,
	})

	response := doJSONRequest(
		t,
		api.client,
		http.MethodPatch,
		api.baseURL+"/v1/companies/"+second.ID.String(),
		fmt.Sprintf(`{"name":%q}`, first.Name),
	)
	assertIntegrationAPIError(t, response, http.StatusConflict, "COMPANY_NAME_CONFLICT")

	persisted := api.getCompany(t, second.ID)
	assertCompanyResponse(t, persisted, second)
}

func TestPatchCompanyNotFound(t *testing.T) {
	api := newIntegrationAPI(t)
	response := doJSONRequest(
		t,
		api.client,
		http.MethodPatch,
		api.baseURL+"/v1/companies/"+uuid.NewString(),
		`{"description":"Updated"}`,
	)

	assertIntegrationAPIError(t, response, http.StatusNotFound, "COMPANY_NOT_FOUND")
}

func TestPatchCompanyValidationPreservesState(t *testing.T) {
	api := newIntegrationAPI(t)
	original := api.createCompany(t, companyAPIRequest{
		Name:              integrationName("valid-"),
		Description:       "Valid company",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeNonProfit,
	})

	response := doJSONRequest(
		t,
		api.client,
		http.MethodPatch,
		api.baseURL+"/v1/companies/"+original.ID.String(),
		`{"name":""}`,
	)
	assertIntegrationAPIError(t, response, http.StatusBadRequest, "INVALID_REQUEST")

	persisted := api.getCompany(t, original.ID)
	assertCompanyResponse(t, persisted, original)
}

func TestDeleteCompanyRemovesPersistedCompany(t *testing.T) {
	api := newIntegrationAPI(t)
	created := api.createCompany(t, companyAPIRequest{
		Name:              integrationName("del-"),
		Description:       "Delete company",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeSoleProprietorship,
	})

	deleteResponse := doJSONRequest(
		t,
		api.client,
		http.MethodDelete,
		api.baseURL+"/v1/companies/"+created.ID.String(),
		"",
	)
	defer closeResponseBody(t, deleteResponse)
	if deleteResponse.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(deleteResponse.Body)
		t.Fatalf("DELETE status = %d, want %d; body = %s", deleteResponse.StatusCode, http.StatusNoContent, body)
	}
	body, err := io.ReadAll(deleteResponse.Body)
	if err != nil {
		t.Fatalf("read DELETE response: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("DELETE body = %q, want empty", body)
	}

	getResponse := doJSONRequest(
		t,
		api.client,
		http.MethodGet,
		api.baseURL+"/v1/companies/"+created.ID.String(),
		"",
	)
	assertIntegrationAPIError(t, getResponse, http.StatusNotFound, "COMPANY_NOT_FOUND")
}

func TestDeleteCompanyNotFound(t *testing.T) {
	api := newIntegrationAPI(t)
	response := doJSONRequest(
		t,
		api.client,
		http.MethodDelete,
		api.baseURL+"/v1/companies/"+uuid.NewString(),
		"",
	)

	assertIntegrationAPIError(t, response, http.StatusNotFound, "COMPANY_NOT_FOUND")
}

type integrationAPI struct {
	pool    *pgxpool.Pool
	client  *http.Client
	baseURL string
}

func newIntegrationAPI(t *testing.T) integrationAPI {
	t.Helper()

	pool := openIntegrationPool(t)
	repository := postgresrepository.NewRepository(pool)
	service := company.NewService(repository, uuid.NewRandom)
	server := httptest.NewServer(httpapi.NewRouter(service, passThroughAuthentication, pool))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = integrationOperationTimeout

	return integrationAPI{
		pool:    pool,
		client:  client,
		baseURL: server.URL,
	}
}

type companyAPIRequest struct {
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	AmountOfEmployees int          `json:"amount_of_employees"`
	Registered        bool         `json:"registered"`
	Type              company.Type `json:"type"`
}

func (api integrationAPI) createCompany(t *testing.T, request companyAPIRequest) companyAPIResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode POST request: %v", err)
	}
	response := doJSONRequest(
		t,
		api.client,
		http.MethodPost,
		api.baseURL+"/v1/companies",
		string(body),
	)
	if response.StatusCode != http.StatusCreated {
		responseBody := readAndClose(t, response)
		t.Fatalf("POST status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, responseBody)
	}
	registerIntegrationCleanup(t, api.pool, request.Name)

	var created companyAPIResponse
	decodeAndClose(t, response, &created)
	if got, want := response.Header.Get("Location"), "/v1/companies/"+created.ID.String(); got != want {
		t.Fatalf("POST Location = %q, want %q", got, want)
	}

	return created
}

func (api integrationAPI) patchCompany(t *testing.T, id uuid.UUID, body string) companyAPIResponse {
	t.Helper()

	response := doJSONRequest(
		t,
		api.client,
		http.MethodPatch,
		api.baseURL+"/v1/companies/"+id.String(),
		body,
	)
	if response.StatusCode != http.StatusOK {
		responseBody := readAndClose(t, response)
		t.Fatalf("PATCH status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, responseBody)
	}

	var updated companyAPIResponse
	decodeAndClose(t, response, &updated)
	return updated
}

func (api integrationAPI) getCompany(t *testing.T, id uuid.UUID) companyAPIResponse {
	t.Helper()

	response := doJSONRequest(
		t,
		api.client,
		http.MethodGet,
		api.baseURL+"/v1/companies/"+id.String(),
		"",
	)
	if response.StatusCode != http.StatusOK {
		responseBody := readAndClose(t, response)
		t.Fatalf("GET status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, responseBody)
	}

	var fetched companyAPIResponse
	decodeAndClose(t, response, &fetched)
	return fetched
}

func integrationName(prefix string) string {
	const (
		maxNameCharacters = 15
		suffixLength      = 8
	)
	if len(prefix)+suffixLength > maxNameCharacters {
		panic("integration company name prefix is too long")
	}

	return prefix + uuid.NewString()[:suffixLength]
}

func assertCompanyResponse(t *testing.T, got, want companyAPIResponse) {
	t.Helper()
	if got != want {
		t.Fatalf("company = %#v, want %#v", got, want)
	}
}

func assertIntegrationAPIError(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	defer closeResponseBody(t, response)

	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, status, body)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q", body.Error.Code, code)
	}
}

type companyAPIResponse struct {
	ID                uuid.UUID    `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	AmountOfEmployees int          `json:"amount_of_employees"`
	Registered        bool         `json:"registered"`
	Type              company.Type `json:"type"`
}

func openIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), integrationOperationTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create integration PostgreSQL pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping integration PostgreSQL: %v", err)
	}

	return pool
}

func registerIntegrationCleanup(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), integrationOperationTimeout)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM companies WHERE name = $1`, name); err != nil {
			t.Errorf("clean up company %q: %v", name, err)
		}
	})
}

func doJSONRequest(t *testing.T, client *http.Client, method, url, body string) *http.Response {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create %s request: %v", method, err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform %s request: %v", method, err)
	}
	return response
}

func passThroughAuthentication(next http.Handler) http.Handler {
	return next
}

func decodeAndClose(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer closeResponseBody(t, response)
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readAndClose(t *testing.T, response *http.Response) string {
	t.Helper()
	defer closeResponseBody(t, response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return string(body)
}

func closeResponseBody(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}
