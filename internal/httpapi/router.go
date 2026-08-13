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
func NewRouter(
	service CompanyService,
	authenticate Middleware,
	readiness ReadinessChecker,
) http.Handler {
	if service == nil {
		panic("httpapi: company service is required")
	}
	if authenticate == nil {
		panic("httpapi: authentication middleware is required")
	}
	if readiness == nil {
		panic("httpapi: readiness checker is required")
	}

	companies := companyHandler{service: service}
	health := healthHandler{readiness: readiness, timeout: readinessTimeout}
	router := http.NewServeMux()
	router.Handle("GET /health/live", http.HandlerFunc(health.live))
	router.Handle("GET /health/ready", http.HandlerFunc(health.ready))
	router.Handle("POST /v1/companies", authenticate(http.HandlerFunc(companies.create)))
	router.Handle("GET /v1/companies/{id}", http.HandlerFunc(companies.get))
	router.Handle("PATCH /v1/companies/{id}", authenticate(http.HandlerFunc(companies.patch)))
	router.Handle("DELETE /v1/companies/{id}", authenticate(http.HandlerFunc(companies.delete)))

	return router
}
