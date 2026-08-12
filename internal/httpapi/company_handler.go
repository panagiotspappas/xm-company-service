package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/google/uuid"
)

const maxRequestBodyBytes int64 = 1 << 20

var (
	errInvalidJSON        = errors.New("invalid JSON request")
	errUnsupportedMedia   = errors.New("unsupported media type")
	errMultipleJSONValues = errors.New("multiple JSON values")
)

type companyHandler struct {
	service CompanyService
}

func (handler companyHandler) create(writer http.ResponseWriter, request *http.Request) {
	if err := validateJSONContentType(request); err != nil {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			errorCodeUnsupportedMediaType,
			"unsupported media type",
		)
		return
	}

	payload, err := decodeJSON[createCompanyRequest](writer, request)
	if err != nil {
		writeDecodeError(writer, err)
		return
	}

	input, err := payload.toInput()
	if err != nil {
		writeError(writer, http.StatusBadRequest, errorCodeInvalidRequest, "invalid request")
		return
	}

	created, err := handler.service.Create(request.Context(), input)
	if err != nil {
		writeServiceError(writer, request.Context(), err)
		return
	}

	writer.Header().Set("Location", "/v1/companies/"+created.ID.String())
	writeJSON(writer, http.StatusCreated, newCompanyResponse(created))
}

func (handler companyHandler) get(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseCompanyID(writer, request)
	if !ok {
		return
	}

	result, err := handler.service.Get(request.Context(), id)
	if err != nil {
		writeServiceError(writer, request.Context(), err)
		return
	}

	writeJSON(writer, http.StatusOK, newCompanyResponse(result))
}

func (handler companyHandler) patch(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseCompanyID(writer, request)
	if !ok {
		return
	}
	if err := validateJSONContentType(request); err != nil {
		writeError(
			writer,
			http.StatusUnsupportedMediaType,
			errorCodeUnsupportedMediaType,
			"unsupported media type",
		)
		return
	}

	payload, err := decodeJSON[patchCompanyRequest](writer, request)
	if err != nil {
		writeDecodeError(writer, err)
		return
	}

	input, err := payload.toInput()
	if err != nil {
		writeError(writer, http.StatusBadRequest, errorCodeInvalidRequest, "invalid request")
		return
	}

	updated, err := handler.service.Patch(request.Context(), id, input)
	if err != nil {
		writeServiceError(writer, request.Context(), err)
		return
	}

	writeJSON(writer, http.StatusOK, newCompanyResponse(updated))
}

func (handler companyHandler) delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseCompanyID(writer, request)
	if !ok {
		return
	}

	if err := handler.service.Delete(request.Context(), id); err != nil {
		writeServiceError(writer, request.Context(), err)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func parseCompanyID(writer http.ResponseWriter, request *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, errorCodeInvalidRequest, "invalid request")
		return uuid.Nil, false
	}

	return id, true
}

func validateJSONContentType(request *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errUnsupportedMedia
	}

	return nil
}

func decodeJSON[T any](writer http.ResponseWriter, request *http.Request) (*T, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	var payload *T
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errInvalidJSON
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, nil
		}
		return nil, err
	}

	return nil, errMultipleJSONValues
}
