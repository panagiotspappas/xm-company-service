package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/panagiotspappas/xm-company-service/internal/company"
)

const (
	uniqueViolationCode  = "23505"
	nameUniqueConstraint = "companies_name_unique"
)

// Repository persists companies in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

var _ company.Repository = (*Repository)(nil)

// NewRepository constructs a PostgreSQL company repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	if pool == nil {
		panic("postgres: pool is required")
	}

	return &Repository{pool: pool}
}

// Create inserts a company.
func (repository *Repository) Create(ctx context.Context, value company.Company) error {
	const query = `
		INSERT INTO companies (
			id,
			name,
			description,
			amount_of_employees,
			registered,
			type
		)
		VALUES ($1, $2, $3, $4, $5, $6)`

	started := time.Now()
	_, err := repository.pool.Exec(
		ctx,
		query,
		value.ID,
		value.Name,
		value.Description,
		value.AmountOfEmployees,
		value.Registered,
		string(value.Type),
	)
	slog.DebugContext(ctx, "database operation completed", "operation", "create_company", "company_id", value.ID, "duration", time.Since(started))
	if err != nil {
		if isNameConflict(err) {
			return fmt.Errorf("insert company: %w", company.ErrNameConflict)
		}
		return fmt.Errorf("insert company: %w", err)
	}

	return nil
}

// GetByID retrieves a company by ID.
func (repository *Repository) GetByID(ctx context.Context, id uuid.UUID) (company.Company, error) {
	const query = `
		SELECT
			id,
			name,
			description,
			amount_of_employees,
			registered,
			type
		FROM companies
		WHERE id = $1`

	var result company.Company
	var companyType string
	started := time.Now()
	err := repository.pool.QueryRow(ctx, query, id).Scan(
		&result.ID,
		&result.Name,
		&result.Description,
		&result.AmountOfEmployees,
		&result.Registered,
		&companyType,
	)
	slog.DebugContext(ctx, "database operation completed", "operation", "get_company", "company_id", id, "duration", time.Since(started))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return company.Company{}, fmt.Errorf("select company: %w", company.ErrNotFound)
		}
		return company.Company{}, fmt.Errorf("select company: %w", err)
	}

	result.Type = company.Type(companyType)
	return result, nil
}

// Update replaces the persisted fields of an existing company.
func (repository *Repository) Update(ctx context.Context, value company.Company) error {
	const query = `
		UPDATE companies
		SET
			name = $1,
			description = $2,
			amount_of_employees = $3,
			registered = $4,
			type = $5
		WHERE id = $6`

	started := time.Now()
	commandTag, err := repository.pool.Exec(
		ctx,
		query,
		value.Name,
		value.Description,
		value.AmountOfEmployees,
		value.Registered,
		string(value.Type),
		value.ID,
	)
	slog.DebugContext(ctx, "database operation completed", "operation", "update_company", "company_id", value.ID, "duration", time.Since(started))
	if err != nil {
		if isNameConflict(err) {
			return fmt.Errorf("update company: %w", company.ErrNameConflict)
		}
		return fmt.Errorf("update company: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("update company: %w", company.ErrNotFound)
	}

	return nil
}

// Delete removes a company by ID.
func (repository *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM companies WHERE id = $1`

	started := time.Now()
	commandTag, err := repository.pool.Exec(ctx, query, id)
	slog.DebugContext(ctx, "database operation completed", "operation", "delete_company", "company_id", id, "duration", time.Since(started))
	if err != nil {
		return fmt.Errorf("delete company: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("delete company: %w", company.ErrNotFound)
	}

	return nil
}

func isNameConflict(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) &&
		postgresErr.Code == uniqueViolationCode &&
		postgresErr.ConstraintName == nameUniqueConstraint
}
