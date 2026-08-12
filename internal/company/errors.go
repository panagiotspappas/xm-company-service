package company

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("company not found")
	ErrNameConflict = errors.New("company name conflict")
)

// ValidationError describes a violation of a company business rule.
type ValidationError struct {
	Field   string
	Message string
}

func (err *ValidationError) Error() string {
	if err.Field == "" {
		return err.Message
	}

	return fmt.Sprintf("%s: %s", err.Field, err.Message)
}
