package company

import "github.com/google/uuid"

// Type identifies the legal form of a company.
type Type string

const (
	TypeCorporations       Type = "Corporations"
	TypeNonProfit          Type = "NonProfit"
	TypeCooperative        Type = "Cooperative"
	TypeSoleProprietorship Type = "Sole Proprietorship"
)

// Company is the complete domain representation of a company.
type Company struct {
	ID                uuid.UUID
	Name              string
	Description       string
	AmountOfEmployees int
	Registered        bool
	Type              Type
}

// CreateCompanyInput contains the values required to create a company.
type CreateCompanyInput struct {
	Name              string
	Description       string
	AmountOfEmployees int
	Registered        bool
	Type              Type
}

// PatchCompanyInput contains only the company fields supplied for an update.
type PatchCompanyInput struct {
	Name              *string
	Description       *string
	AmountOfEmployees *int
	Registered        *bool
	Type              *Type
}

func (input PatchCompanyInput) empty() bool {
	return input.Name == nil &&
		input.Description == nil &&
		input.AmountOfEmployees == nil &&
		input.Registered == nil &&
		input.Type == nil
}
