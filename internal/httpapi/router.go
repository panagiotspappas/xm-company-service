package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/panagiotspappas/xm-company-service/internal/company"
)

// CompanyService defines the application behavior consumed by the HTTP API.
type CompanyService interface {
	Create(context.Context, company.CreateCompanyInput) (company.Company, error)
	Get(context.Context, uuid.UUID) (company.Company, error)
	Patch(context.Context, uuid.UUID, company.PatchCompanyInput) (company.Company, error)
	Delete(context.Context, uuid.UUID) error
}

// NewRouter constructs the company HTTP API.
func NewRouter(service CompanyService) http.Handler {
	if service == nil {
		panic("httpapi: company service is required")
	}

	handler := companyHandler{service: service}
	router := http.NewServeMux()
	router.HandleFunc("POST /v1/companies", handler.create)
	router.HandleFunc("GET /v1/companies/{id}", handler.get)
	router.HandleFunc("PATCH /v1/companies/{id}", handler.patch)
	router.HandleFunc("DELETE /v1/companies/{id}", handler.delete)

	return router
}
