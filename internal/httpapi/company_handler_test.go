package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/panagiotspappas/xm-company-service/internal/company"
)

func TestNewRouterRequiresService(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("NewRouter did not panic")
		}
	}()

	NewRouter(nil, passThroughAuthentication)
}

func TestCreateCompany(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("11f8da54-0659-44a8-b0e8-4717f7f245ce")
	wantInput := company.CreateCompanyInput{
		Name:              "Acme",
		Description:       "",
		AmountOfEmployees: 0,
		Registered:        false,
		Type:              company.TypeCorporations,
	}
	wantCompany := company.Company{
		ID:                id,
		Name:              wantInput.Name,
		Description:       wantInput.Description,
		AmountOfEmployees: wantInput.AmountOfEmployees,
		Registered:        wantInput.Registered,
		Type:              wantInput.Type,
	}
	service := &fakeCompanyService{
		create: func(_ context.Context, input company.CreateCompanyInput) (company.Company, error) {
			if !reflect.DeepEqual(input, wantInput) {
				t.Fatalf("Create() input = %#v, want %#v", input, wantInput)
			}
			return wantCompany, nil
		},
	}

	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodPost,
		"/v1/companies",
		`{"name":"Acme","amount_of_employees":0,"registered":false,"type":"Corporations"}`,
		"application/json; charset=utf-8",
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got, want := response.Header().Get("Location"), "/v1/companies/"+id.String(); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}

	var got companyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := newCompanyResponse(wantCompany); !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestCreateCompanyRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	validFields := `"name":"Acme","amount_of_employees":1,"registered":true,"type":"Corporations"`
	tests := map[string]string{
		"empty":               "",
		"top-level null":      "null",
		"malformed":           "{",
		"unknown field":       `{` + validFields + `,"unknown":true}`,
		"multiple values":     `{` + validFields + `} {}`,
		"wrong field type":    `{` + validFields + `,"description":42}`,
		"missing name":        `{"amount_of_employees":1,"registered":true,"type":"Corporations"}`,
		"missing employees":   `{"name":"Acme","registered":true,"type":"Corporations"}`,
		"missing registered":  `{"name":"Acme","amount_of_employees":1,"type":"Corporations"}`,
		"missing type":        `{"name":"Acme","amount_of_employees":1,"registered":true}`,
		"null name":           `{` + validFields + `,"name":null}`,
		"null description":    `{` + validFields + `,"description":null}`,
		"null employee count": `{"name":"Acme","amount_of_employees":null,"registered":true,"type":"Corporations"}`,
		"null registration":   `{"name":"Acme","amount_of_employees":1,"registered":null,"type":"Corporations"}`,
		"null company type":   `{"name":"Acme","amount_of_employees":1,"registered":true,"type":null}`,
	}

	for name, body := range tests {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			createCalls := 0
			service := &fakeCompanyService{
				create: func(context.Context, company.CreateCompanyInput) (company.Company, error) {
					createCalls++
					return company.Company{}, nil
				},
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodPost,
				"/v1/companies",
				body,
				"application/json",
			)

			assertAPIError(t, response, http.StatusBadRequest, errorCodeInvalidRequest)
			if createCalls != 0 {
				t.Fatalf("Create() calls = %d, want 0", createCalls)
			}
		})
	}
}

func TestCreateCompanyContentType(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing":     "",
		"malformed":   "application/json; =utf-8",
		"unsupported": "text/plain",
	}
	for name, contentType := range tests {
		contentType := contentType
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := &fakeCompanyService{}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodPost,
				"/v1/companies",
				`{}`,
				contentType,
			)
			assertAPIError(t, response, http.StatusUnsupportedMediaType, errorCodeUnsupportedMediaType)
		})
	}
}

func TestCreateCompanyRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	service := &fakeCompanyService{}
	body := `{"name":"` + strings.Repeat("a", int(maxRequestBodyBytes)) + `"}`
	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodPost,
		"/v1/companies",
		body,
		"application/json",
	)

	assertAPIError(t, response, http.StatusRequestEntityTooLarge, errorCodeContentTooLarge)
}

func TestCreateCompanyMapsServiceErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err       error
		status    int
		errorCode string
	}{
		"validation": {
			err:       &company.ValidationError{Field: "name", Message: "invalid"},
			status:    http.StatusBadRequest,
			errorCode: errorCodeInvalidRequest,
		},
		"wrapped conflict": {
			err:       errors.Join(errors.New("create"), company.ErrNameConflict),
			status:    http.StatusConflict,
			errorCode: errorCodeCompanyNameConflict,
		},
		"internal": {
			err:       errors.New("unexpected"),
			status:    http.StatusInternalServerError,
			errorCode: errorCodeInternal,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := &fakeCompanyService{
				create: func(context.Context, company.CreateCompanyInput) (company.Company, error) {
					return company.Company{}, test.err
				},
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodPost,
				"/v1/companies",
				`{"name":"Acme","amount_of_employees":1,"registered":true,"type":"Corporations"}`,
				"application/json",
			)
			assertAPIError(t, response, test.status, test.errorCode)
		})
	}
}

func TestGetCompany(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	want := testCompany(id)
	service := &fakeCompanyService{
		get: func(_ context.Context, gotID uuid.UUID) (company.Company, error) {
			if gotID != id {
				t.Fatalf("Get() ID = %s, want %s", gotID, id)
			}
			return want, nil
		},
	}
	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodGet,
		"/v1/companies/"+id.String(),
		"",
		"",
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestGetCompanyRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	getCalls := 0
	service := &fakeCompanyService{
		get: func(context.Context, uuid.UUID) (company.Company, error) {
			getCalls++
			return company.Company{}, nil
		},
	}
	response := performRequest(t, newTestRouter(service), http.MethodGet, "/v1/companies/not-a-uuid", "", "")

	assertAPIError(t, response, http.StatusBadRequest, errorCodeInvalidRequest)
	if getCalls != 0 {
		t.Fatalf("Get() calls = %d, want 0", getCalls)
	}
}

func TestGetCompanyMapsServiceErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err       error
		status    int
		errorCode string
	}{
		"wrapped not found": {
			err:       errors.Join(errors.New("get"), company.ErrNotFound),
			status:    http.StatusNotFound,
			errorCode: errorCodeCompanyNotFound,
		},
		"internal": {
			err:       errors.New("unexpected"),
			status:    http.StatusInternalServerError,
			errorCode: errorCodeInternal,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := &fakeCompanyService{
				get: func(context.Context, uuid.UUID) (company.Company, error) {
					return company.Company{}, test.err
				},
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodGet,
				"/v1/companies/"+uuid.NewString(),
				"",
				"",
			)
			assertAPIError(t, response, test.status, test.errorCode)
		})
	}
}

func TestPatchCompany(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	wantCompany := testCompany(id)
	patchCalls := 0
	service := &fakeCompanyService{
		patch: func(_ context.Context, gotID uuid.UUID, input company.PatchCompanyInput) (company.Company, error) {
			patchCalls++
			if gotID != id {
				t.Fatalf("Patch() ID = %s, want %s", gotID, id)
			}
			if input.Name != nil || input.Type != nil {
				t.Fatalf("omitted fields were populated: %#v", input)
			}
			if input.Description == nil || *input.Description != "" {
				t.Fatalf("Description = %#v, want pointer to empty string", input.Description)
			}
			if input.AmountOfEmployees == nil || *input.AmountOfEmployees != 0 {
				t.Fatalf("AmountOfEmployees = %#v, want pointer to 0", input.AmountOfEmployees)
			}
			if input.Registered == nil || *input.Registered {
				t.Fatalf("Registered = %#v, want pointer to false", input.Registered)
			}
			return wantCompany, nil
		},
	}
	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodPatch,
		"/v1/companies/"+id.String(),
		`{"description":"","amount_of_employees":0,"registered":false}`,
		"application/json",
	)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
	}
	if patchCalls != 1 {
		t.Fatalf("Patch() calls = %d, want 1", patchCalls)
	}
}

func TestPatchCompanyRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":            "",
		"top-level null":   "null",
		"empty object":     `{}`,
		"malformed":        `{`,
		"unknown field":    `{"unknown":true}`,
		"multiple values":  `{"name":"Acme"} {}`,
		"wrong field type": `{"registered":"false"}`,
		"null description": `{"description":null}`,
	}

	for name, body := range tests {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			patchCalls := 0
			service := &fakeCompanyService{
				patch: func(context.Context, uuid.UUID, company.PatchCompanyInput) (company.Company, error) {
					patchCalls++
					return company.Company{}, nil
				},
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodPatch,
				"/v1/companies/"+uuid.NewString(),
				body,
				"application/json",
			)

			assertAPIError(t, response, http.StatusBadRequest, errorCodeInvalidRequest)
			if patchCalls != 0 {
				t.Fatalf("Patch() calls = %d, want 0", patchCalls)
			}
		})
	}
}

func TestPatchCompanyRejectsInvalidContentTypeAndOversizedBody(t *testing.T) {
	t.Parallel()

	contentTypes := map[string]string{
		"missing":     "",
		"malformed":   "application/json; =utf-8",
		"unsupported": "text/plain",
	}
	for name, contentType := range contentTypes {
		contentType := contentType
		t.Run(name+" content type", func(t *testing.T) {
			t.Parallel()
			response := performRequest(
				t,
				newTestRouter(&fakeCompanyService{}),
				http.MethodPatch,
				"/v1/companies/"+uuid.NewString(),
				`{}`,
				contentType,
			)
			assertAPIError(t, response, http.StatusUnsupportedMediaType, errorCodeUnsupportedMediaType)
		})
	}

	t.Run("oversized body", func(t *testing.T) {
		t.Parallel()
		body := `{"description":"` + strings.Repeat("a", int(maxRequestBodyBytes)) + `"}`
		response := performRequest(
			t,
			newTestRouter(&fakeCompanyService{}),
			http.MethodPatch,
			"/v1/companies/"+uuid.NewString(),
			body,
			"application/json",
		)
		assertAPIError(t, response, http.StatusRequestEntityTooLarge, errorCodeContentTooLarge)
	})
}

func TestPatchCompanyRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	patchCalls := 0
	service := &fakeCompanyService{
		patch: func(context.Context, uuid.UUID, company.PatchCompanyInput) (company.Company, error) {
			patchCalls++
			return company.Company{}, nil
		},
	}
	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodPatch,
		"/v1/companies/not-a-uuid",
		`{"name":"Other"}`,
		"application/json",
	)

	assertAPIError(t, response, http.StatusBadRequest, errorCodeInvalidRequest)
	if patchCalls != 0 {
		t.Fatalf("Patch() calls = %d, want 0", patchCalls)
	}
}

func TestPatchCompanyMapsServiceErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err       error
		status    int
		errorCode string
	}{
		"validation": {
			err:       &company.ValidationError{Field: "name", Message: "invalid"},
			status:    http.StatusBadRequest,
			errorCode: errorCodeInvalidRequest,
		},
		"not found": {
			err:       errors.Join(errors.New("patch"), company.ErrNotFound),
			status:    http.StatusNotFound,
			errorCode: errorCodeCompanyNotFound,
		},
		"conflict": {
			err:       errors.Join(errors.New("patch"), company.ErrNameConflict),
			status:    http.StatusConflict,
			errorCode: errorCodeCompanyNameConflict,
		},
		"internal": {
			err:       errors.New("unexpected"),
			status:    http.StatusInternalServerError,
			errorCode: errorCodeInternal,
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service := &fakeCompanyService{
				patch: func(context.Context, uuid.UUID, company.PatchCompanyInput) (company.Company, error) {
					return company.Company{}, test.err
				},
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodPatch,
				"/v1/companies/"+uuid.NewString(),
				`{"name":"Other"}`,
				"application/json",
			)
			assertAPIError(t, response, test.status, test.errorCode)
		})
	}
}

func TestDeleteCompany(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	service := &fakeCompanyService{
		delete: func(_ context.Context, gotID uuid.UUID) error {
			if gotID != id {
				t.Fatalf("Delete() ID = %s, want %s", gotID, id)
			}
			return nil
		},
	}
	response := performRequest(
		t,
		newTestRouter(service),
		http.MethodDelete,
		"/v1/companies/"+id.String(),
		"",
		"",
	)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func TestDeleteCompanyErrors(t *testing.T) {
	t.Parallel()

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()
		deleteCalls := 0
		service := &fakeCompanyService{
			delete: func(context.Context, uuid.UUID) error {
				deleteCalls++
				return nil
			},
		}
		response := performRequest(t, newTestRouter(service), http.MethodDelete, "/v1/companies/invalid", "", "")
		assertAPIError(t, response, http.StatusBadRequest, errorCodeInvalidRequest)
		if deleteCalls != 0 {
			t.Fatalf("Delete() calls = %d, want 0", deleteCalls)
		}
	})

	tests := map[string]struct {
		err       error
		status    int
		errorCode string
	}{
		"not found": {
			err:       errors.Join(errors.New("delete"), company.ErrNotFound),
			status:    http.StatusNotFound,
			errorCode: errorCodeCompanyNotFound,
		},
		"internal": {
			err:       errors.New("unexpected"),
			status:    http.StatusInternalServerError,
			errorCode: errorCodeInternal,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			service := &fakeCompanyService{
				delete: func(context.Context, uuid.UUID) error { return test.err },
			}
			response := performRequest(
				t,
				newTestRouter(service),
				http.MethodDelete,
				"/v1/companies/"+uuid.NewString(),
				"",
				"",
			)
			assertAPIError(t, response, test.status, test.errorCode)
		})
	}
}

type fakeCompanyService struct {
	create func(context.Context, company.CreateCompanyInput) (company.Company, error)
	get    func(context.Context, uuid.UUID) (company.Company, error)
	patch  func(context.Context, uuid.UUID, company.PatchCompanyInput) (company.Company, error)
	delete func(context.Context, uuid.UUID) error
}

func newTestRouter(service CompanyService) http.Handler {
	return NewRouter(service, passThroughAuthentication)
}

func passThroughAuthentication(next http.Handler) http.Handler {
	return next
}

func (service *fakeCompanyService) Create(
	ctx context.Context,
	input company.CreateCompanyInput,
) (company.Company, error) {
	if service.create == nil {
		return company.Company{}, nil
	}
	return service.create(ctx, input)
}

func (service *fakeCompanyService) Get(ctx context.Context, id uuid.UUID) (company.Company, error) {
	if service.get == nil {
		return company.Company{}, nil
	}
	return service.get(ctx, id)
}

func (service *fakeCompanyService) Patch(
	ctx context.Context,
	id uuid.UUID,
	input company.PatchCompanyInput,
) (company.Company, error) {
	if service.patch == nil {
		return company.Company{}, nil
	}
	return service.patch(ctx, id, input)
}

func (service *fakeCompanyService) Delete(ctx context.Context, id uuid.UUID) error {
	if service.delete == nil {
		return nil
	}
	return service.delete(ctx, id)
}

func performRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body string,
	contentType string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()

	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q", body.Error.Code, code)
	}
	if body.Error.Message == "" {
		t.Fatal("error message is empty")
	}
}

func testCompany(id uuid.UUID) company.Company {
	return company.Company{
		ID:                id,
		Name:              "Acme",
		Description:       "Software",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeCorporations,
	}
}
