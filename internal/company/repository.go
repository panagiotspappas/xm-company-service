package company

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the persistence behavior required by Service.
type Repository interface {
	Create(ctx context.Context, company Company) error
	GetByID(ctx context.Context, id uuid.UUID) (Company, error)
	Update(ctx context.Context, company Company) error
	Delete(ctx context.Context, id uuid.UUID) error
}
