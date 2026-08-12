//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/panagiotspappas/xm-company-service/internal/company"
)

const testOperationTimeout = 5 * time.Second

func TestNewRepositoryRequiresPool(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewRepository did not panic")
		}
	}()

	NewRepository(nil)
}

func TestRepositoryCreateAndGetByID(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	want := testCompany(shortName("create-"))
	want.Description = ""
	want.AmountOfEmployees = 0
	want.Registered = false

	createCompany(t, repository, pool, want)

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	got, err := repository.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetByID() = %#v, want %#v", got, want)
	}
}

func TestRepositoryGetByIDNotFound(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	_, err := repository.GetByID(ctx, uuid.New())
	if !errors.Is(err, company.ErrNotFound) {
		t.Fatalf("GetByID() error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryCreateMapsNameConflict(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	name := shortName("dup-")
	first := testCompany(name)
	createCompany(t, repository, pool, first)

	duplicate := testCompany(name)
	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	err := repository.Create(ctx, duplicate)
	if !errors.Is(err, company.ErrNameConflict) {
		t.Fatalf("Create() error = %v, want ErrNameConflict", err)
	}
}

func TestRepositoryNameUniquenessIsCaseSensitive(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	suffix := uuid.NewString()[:8]
	first := testCompany("Ac" + suffix)
	second := testCompany("aC" + suffix)

	createCompany(t, repository, pool, first)
	createCompany(t, repository, pool, second)
}

func TestRepositoryUpdate(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	original := testCompany(shortName("old-"))
	createCompany(t, repository, pool, original)

	updated := original
	updated.Name = shortName("new-")
	updated.Description = "Updated"
	updated.AmountOfEmployees = 0
	updated.Registered = false
	updated.Type = company.TypeCooperative

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	if err := repository.Update(ctx, updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := repository.GetByID(ctx, updated.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Fatalf("updated company = %#v, want %#v", got, updated)
	}
}

func TestRepositoryUpdateNotFound(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	missing := testCompany(shortName("miss-"))

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	err := repository.Update(ctx, missing)
	if !errors.Is(err, company.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryUpdateMapsNameConflict(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	first := testCompany(shortName("first-"))
	second := testCompany(shortName("second-"))
	createCompany(t, repository, pool, first)
	createCompany(t, repository, pool, second)

	second.Name = first.Name
	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	err := repository.Update(ctx, second)
	if !errors.Is(err, company.ErrNameConflict) {
		t.Fatalf("Update() error = %v, want ErrNameConflict", err)
	}
}

func TestRepositoryDelete(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	value := testCompany(shortName("delete-"))
	createCompany(t, repository, pool, value)

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	if err := repository.Delete(ctx, value.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err := repository.GetByID(ctx, value.ID)
	if !errors.Is(err, company.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryDeleteNotFound(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	err := repository.Delete(ctx, uuid.New())
	if !errors.Is(err, company.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test PostgreSQL pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test PostgreSQL: %v", err)
	}

	return pool
}

func createCompany(t *testing.T, repository *Repository, pool *pgxpool.Pool, value company.Company) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
	defer cancel()
	if err := repository.Create(ctx, value); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	registerCompanyCleanup(t, pool, value.ID)
}

func registerCompanyCleanup(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testOperationTimeout)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM companies WHERE id = $1`, id); err != nil {
			t.Errorf("clean up company %s: %v", id, err)
		}
	})
}

func testCompany(name string) company.Company {
	return company.Company{
		ID:                uuid.New(),
		Name:              name,
		Description:       "Test company",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              company.TypeCorporations,
	}
}

func shortName(prefix string) string {
	const (
		maxNameCharacters = 15
		suffixLength      = 8
	)
	if len(prefix)+suffixLength > maxNameCharacters {
		panic("test company name prefix is too long")
	}

	return prefix + uuid.NewString()[:suffixLength]
}
