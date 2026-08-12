package company

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxNameCharacters        = 15
	maxDescriptionCharacters = 3000
)

// IDGenerator generates company identifiers.
type IDGenerator func() (uuid.UUID, error)

// Service implements company application behavior.
type Service struct {
	repository Repository
	generateID IDGenerator
}

// NewService constructs a company service from its mandatory dependencies.
func NewService(repository Repository, generateID IDGenerator) *Service {
	if repository == nil {
		panic("company: repository is required")
	}
	if generateID == nil {
		panic("company: ID generator is required")
	}

	return &Service{
		repository: repository,
		generateID: generateID,
	}
}

// Create creates and persists a validated company.
func (service *Service) Create(ctx context.Context, input CreateCompanyInput) (Company, error) {
	created := Company{
		Name:              input.Name,
		Description:       input.Description,
		AmountOfEmployees: input.AmountOfEmployees,
		Registered:        input.Registered,
		Type:              input.Type,
	}
	if err := validateCompany(created); err != nil {
		return Company{}, err
	}

	id, err := service.generateID()
	if err != nil {
		return Company{}, fmt.Errorf("generate company ID: %w", err)
	}
	created.ID = id

	if err := service.repository.Create(ctx, created); err != nil {
		return Company{}, fmt.Errorf("create company: %w", err)
	}

	return created, nil
}

// Get retrieves a company by ID.
func (service *Service) Get(ctx context.Context, id uuid.UUID) (Company, error) {
	result, err := service.repository.GetByID(ctx, id)
	if err != nil {
		return Company{}, fmt.Errorf("get company: %w", err)
	}

	return result, nil
}

// Patch applies a partial update to an existing company.
func (service *Service) Patch(
	ctx context.Context,
	id uuid.UUID,
	input PatchCompanyInput,
) (Company, error) {
	if input.empty() {
		return Company{}, &ValidationError{
			Field:   "patch",
			Message: "at least one field is required",
		}
	}

	current, err := service.repository.GetByID(ctx, id)
	if err != nil {
		return Company{}, fmt.Errorf("get company for patch: %w", err)
	}

	applyPatch(&current, input)
	if err := validateCompany(current); err != nil {
		return Company{}, err
	}

	if err := service.repository.Update(ctx, current); err != nil {
		return Company{}, fmt.Errorf("update company: %w", err)
	}

	return current, nil
}

// Delete removes a company by ID.
func (service *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := service.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete company: %w", err)
	}

	return nil
}

func validateCompany(value Company) error {
	if strings.TrimSpace(value.Name) == "" {
		return &ValidationError{Field: "name", Message: "must not be blank"}
	}
	if utf8.RuneCountInString(value.Name) > maxNameCharacters {
		return &ValidationError{Field: "name", Message: "must not exceed 15 characters"}
	}
	if utf8.RuneCountInString(value.Description) > maxDescriptionCharacters {
		return &ValidationError{
			Field:   "description",
			Message: "must not exceed 3000 characters",
		}
	}
	if value.AmountOfEmployees < 0 {
		return &ValidationError{
			Field:   "amount_of_employees",
			Message: "must be greater than or equal to zero",
		}
	}
	if !validType(value.Type) {
		return &ValidationError{Field: "type", Message: "is invalid"}
	}

	return nil
}

func validType(value Type) bool {
	switch value {
	case TypeCorporations, TypeNonProfit, TypeCooperative, TypeSoleProprietorship:
		return true
	default:
		return false
	}
}

func applyPatch(target *Company, input PatchCompanyInput) {
	if input.Name != nil {
		target.Name = *input.Name
	}
	if input.Description != nil {
		target.Description = *input.Description
	}
	if input.AmountOfEmployees != nil {
		target.AmountOfEmployees = *input.AmountOfEmployees
	}
	if input.Registered != nil {
		target.Registered = *input.Registered
	}
	if input.Type != nil {
		target.Type = *input.Type
	}
}
