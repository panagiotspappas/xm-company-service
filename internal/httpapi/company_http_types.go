package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/panagiotspappas/xm-company-service/internal/company"
)

var (
	errNullNotAllowed = errors.New("null is not allowed")
	errMissingField   = errors.New("required field is missing")
	errEmptyPatch     = errors.New("at least one patch field is required")
)

type optional[T any] struct {
	Value T
	Set   bool
}

func (value *optional[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errNullNotAllowed
	}

	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	value.Value = decoded
	value.Set = true
	return nil
}

type createCompanyRequest struct {
	Name              optional[string]       `json:"name"`
	Description       optional[string]       `json:"description"`
	AmountOfEmployees optional[int]          `json:"amount_of_employees"`
	Registered        optional[bool]         `json:"registered"`
	Type              optional[company.Type] `json:"type"`
}

func (request createCompanyRequest) toInput() (company.CreateCompanyInput, error) {
	if !request.Name.Set || !request.AmountOfEmployees.Set || !request.Registered.Set || !request.Type.Set {
		return company.CreateCompanyInput{}, errMissingField
	}

	description := ""
	if request.Description.Set {
		description = request.Description.Value
	}

	return company.CreateCompanyInput{
		Name:              request.Name.Value,
		Description:       description,
		AmountOfEmployees: request.AmountOfEmployees.Value,
		Registered:        request.Registered.Value,
		Type:              request.Type.Value,
	}, nil
}

type patchCompanyRequest struct {
	Name              optional[string]       `json:"name"`
	Description       optional[string]       `json:"description"`
	AmountOfEmployees optional[int]          `json:"amount_of_employees"`
	Registered        optional[bool]         `json:"registered"`
	Type              optional[company.Type] `json:"type"`
}

func (request patchCompanyRequest) toInput() (company.PatchCompanyInput, error) {
	if !request.Name.Set &&
		!request.Description.Set &&
		!request.AmountOfEmployees.Set &&
		!request.Registered.Set &&
		!request.Type.Set {
		return company.PatchCompanyInput{}, errEmptyPatch
	}

	var input company.PatchCompanyInput
	if request.Name.Set {
		input.Name = &request.Name.Value
	}
	if request.Description.Set {
		input.Description = &request.Description.Value
	}
	if request.AmountOfEmployees.Set {
		input.AmountOfEmployees = &request.AmountOfEmployees.Value
	}
	if request.Registered.Set {
		input.Registered = &request.Registered.Value
	}
	if request.Type.Set {
		input.Type = &request.Type.Value
	}

	return input, nil
}

type companyResponse struct {
	ID                uuid.UUID    `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	AmountOfEmployees int          `json:"amount_of_employees"`
	Registered        bool         `json:"registered"`
	Type              company.Type `json:"type"`
}

func newCompanyResponse(value company.Company) companyResponse {
	return companyResponse{
		ID:                value.ID,
		Name:              value.Name,
		Description:       value.Description,
		AmountOfEmployees: value.AmountOfEmployees,
		Registered:        value.Registered,
		Type:              value.Type,
	}
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
