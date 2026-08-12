package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/panagiotspappas/xm-company-service/internal/company"
)

const (
	errorCodeInvalidRequest       = "INVALID_REQUEST"
	errorCodeCompanyNotFound      = "COMPANY_NOT_FOUND"
	errorCodeCompanyNameConflict  = "COMPANY_NAME_CONFLICT"
	errorCodeContentTooLarge      = "CONTENT_TOO_LARGE"
	errorCodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
	errorCodeUnauthorized         = "UNAUTHORIZED"
	errorCodeServiceUnavailable   = "SERVICE_UNAVAILABLE"
	errorCodeInternal             = "INTERNAL_ERROR"
)

func writeServiceError(writer http.ResponseWriter, ctx context.Context, err error) {
	var validationErr *company.ValidationError

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		writeError(
			writer,
			http.StatusServiceUnavailable,
			errorCodeServiceUnavailable,
			"service unavailable",
		)
	case ctx.Err() == context.Canceled:
		return
	case errors.As(err, &validationErr):
		writeError(writer, http.StatusBadRequest, errorCodeInvalidRequest, "invalid request")
	case errors.Is(err, company.ErrNotFound):
		writeError(writer, http.StatusNotFound, errorCodeCompanyNotFound, "company not found")
	case errors.Is(err, company.ErrNameConflict):
		writeError(writer, http.StatusConflict, errorCodeCompanyNameConflict, "company name conflict")
	default:
		writeError(writer, http.StatusInternalServerError, errorCodeInternal, "internal server error")
	}
}

func writeDecodeError(writer http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(
			writer,
			http.StatusRequestEntityTooLarge,
			errorCodeContentTooLarge,
			"request body is too large",
		)
		return
	}

	writeError(writer, http.StatusBadRequest, errorCodeInvalidRequest, "invalid request")
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
