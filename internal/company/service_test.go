package company

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewServiceRequiresDependencies(t *testing.T) {
	t.Parallel()

	tests := map[string]func(){
		"repository":   func() { NewService(nil, uuid.NewRandom) },
		"ID generator": func() { NewService(&fakeRepository{}, nil) },
	}

	for name, construct := range tests {
		construct := construct
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Fatal("NewService did not panic")
				}
			}()

			construct()
		})
	}
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("4ca0c726-cfd2-47d6-bd8e-31b65d86aabd")
	input := validCreateInput()
	var persisted Company
	repository := &fakeRepository{
		create: func(_ context.Context, value Company) error {
			persisted = value
			return nil
		},
	}
	service := NewService(repository, func() (uuid.UUID, error) { return id, nil })

	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := Company{
		ID:                id,
		Name:              input.Name,
		Description:       input.Description,
		AmountOfEmployees: input.AmountOfEmployees,
		Registered:        input.Registered,
		Type:              input.Type,
	}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("Create() = %#v, want %#v", created, want)
	}
	if !reflect.DeepEqual(persisted, want) {
		t.Fatalf("persisted company = %#v, want %#v", persisted, want)
	}
}

func TestServiceCreateValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*CreateCompanyInput){
		"blank name": func(input *CreateCompanyInput) { input.Name = "\t \u2003" },
		"long Unicode name": func(input *CreateCompanyInput) {
			input.Name = strings.Repeat("界", maxNameCharacters+1)
		},
		"long Unicode description": func(input *CreateCompanyInput) {
			input.Description = strings.Repeat("界", maxDescriptionCharacters+1)
		},
		"negative employees": func(input *CreateCompanyInput) { input.AmountOfEmployees = -1 },
		"invalid type":       func(input *CreateCompanyInput) { input.Type = Type("Partnership") },
	}

	for name, mutate := range tests {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := validCreateInput()
			mutate(&input)
			generatorCalls := 0
			createCalls := 0
			service := NewService(&fakeRepository{
				create: func(context.Context, Company) error {
					createCalls++
					return nil
				},
			}, func() (uuid.UUID, error) {
				generatorCalls++
				return uuid.New(), nil
			})

			_, err := service.Create(context.Background(), input)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("Create() error = %v, want *ValidationError", err)
			}
			if generatorCalls != 0 {
				t.Fatalf("ID generator calls = %d, want 0", generatorCalls)
			}
			if createCalls != 0 {
				t.Fatalf("Repository.Create calls = %d, want 0", createCalls)
			}
		})
	}
}

func TestServiceCreateAcceptsZeroValues(t *testing.T) {
	t.Parallel()

	input := validCreateInput()
	input.AmountOfEmployees = 0
	input.Registered = false
	service := NewService(&fakeRepository{}, uuid.NewRandom)

	if _, err := service.Create(context.Background(), input); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestServiceCreateIDGenerationFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("entropy unavailable")
	createCalls := 0
	service := NewService(&fakeRepository{
		create: func(context.Context, Company) error {
			createCalls++
			return nil
		},
	}, func() (uuid.UUID, error) {
		return uuid.Nil, wantErr
	})

	_, err := service.Create(context.Background(), validCreateInput())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Create() error = %v, want wrapped %v", err, wantErr)
	}
	if createCalls != 0 {
		t.Fatalf("Repository.Create calls = %d, want 0", createCalls)
	}
}

func TestServiceCreatePropagatesConflict(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{
		create: func(context.Context, Company) error {
			return errors.Join(errors.New("database"), ErrNameConflict)
		},
	}, uuid.NewRandom)

	_, err := service.Create(context.Background(), validCreateInput())
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("Create() error = %v, want wrapped ErrNameConflict", err)
	}
}

func TestServiceGet(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	want := validCompany(id)
	service := NewService(&fakeRepository{
		getByID: func(_ context.Context, gotID uuid.UUID) (Company, error) {
			if gotID != id {
				t.Fatalf("GetByID() ID = %s, want %s", gotID, id)
			}
			return want, nil
		},
	}, uuid.NewRandom)

	got, err := service.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}

func TestServiceGetPropagatesWrappedNotFound(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{
		getByID: func(context.Context, uuid.UUID) (Company, error) {
			return Company{}, errors.Join(errors.New("query"), ErrNotFound)
		},
	}, uuid.NewRandom)

	_, err := service.Get(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want wrapped ErrNotFound", err)
	}
}

func TestServicePatchRejectsEmptyInput(t *testing.T) {
	t.Parallel()

	getCalls := 0
	updateCalls := 0
	service := NewService(&fakeRepository{
		getByID: func(context.Context, uuid.UUID) (Company, error) {
			getCalls++
			return Company{}, nil
		},
		update: func(context.Context, Company) error {
			updateCalls++
			return nil
		},
	}, uuid.NewRandom)

	_, err := service.Patch(context.Background(), uuid.New(), PatchCompanyInput{})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Patch() error = %v, want *ValidationError", err)
	}
	if getCalls != 0 || updateCalls != 0 {
		t.Fatalf("repository calls = get %d, update %d; want 0, 0", getCalls, updateCalls)
	}
}

func TestServicePatchMergesSuppliedFields(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	current := validCompany(id)
	name := "New Name"
	description := ""
	employees := 0
	registered := false
	typeValue := TypeCooperative
	input := PatchCompanyInput{
		Name:              &name,
		Description:       &description,
		AmountOfEmployees: &employees,
		Registered:        &registered,
		Type:              &typeValue,
	}
	var persisted Company
	service := NewService(&fakeRepository{
		getByID: func(context.Context, uuid.UUID) (Company, error) { return current, nil },
		update: func(_ context.Context, value Company) error {
			persisted = value
			return nil
		},
	}, uuid.NewRandom)

	updated, err := service.Patch(context.Background(), id, input)
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	want := Company{
		ID:                id,
		Name:              name,
		Description:       description,
		AmountOfEmployees: employees,
		Registered:        registered,
		Type:              typeValue,
	}
	if !reflect.DeepEqual(updated, want) {
		t.Fatalf("Patch() = %#v, want %#v", updated, want)
	}
	if !reflect.DeepEqual(persisted, want) {
		t.Fatalf("persisted company = %#v, want %#v", persisted, want)
	}
}

func TestServicePatchPreservesOmittedFields(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	current := validCompany(id)
	registered := false
	var persisted Company
	service := NewService(&fakeRepository{
		getByID: func(context.Context, uuid.UUID) (Company, error) { return current, nil },
		update: func(_ context.Context, value Company) error {
			persisted = value
			return nil
		},
	}, uuid.NewRandom)

	updated, err := service.Patch(
		context.Background(),
		id,
		PatchCompanyInput{Registered: &registered},
	)
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}

	want := current
	want.Registered = false
	if !reflect.DeepEqual(updated, want) {
		t.Fatalf("Patch() = %#v, want %#v", updated, want)
	}
	if !reflect.DeepEqual(persisted, want) {
		t.Fatalf("persisted company = %#v, want %#v", persisted, want)
	}
}

func TestServicePatchValidationPreventsUpdate(t *testing.T) {
	t.Parallel()

	blank := " "
	updateCalls := 0
	service := NewService(&fakeRepository{
		getByID: func(context.Context, uuid.UUID) (Company, error) {
			return validCompany(uuid.New()), nil
		},
		update: func(context.Context, Company) error {
			updateCalls++
			return nil
		},
	}, uuid.NewRandom)

	_, err := service.Patch(context.Background(), uuid.New(), PatchCompanyInput{Name: &blank})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Patch() error = %v, want *ValidationError", err)
	}
	if updateCalls != 0 {
		t.Fatalf("Repository.Update calls = %d, want 0", updateCalls)
	}
}

func TestServicePatchPropagatesRepositoryErrors(t *testing.T) {
	t.Parallel()

	name := "Other"
	tests := map[string]struct {
		repository *fakeRepository
		wantErr    error
	}{
		"not found": {
			repository: &fakeRepository{
				getByID: func(context.Context, uuid.UUID) (Company, error) {
					return Company{}, errors.Join(errors.New("query"), ErrNotFound)
				},
			},
			wantErr: ErrNotFound,
		},
		"name conflict": {
			repository: &fakeRepository{
				getByID: func(context.Context, uuid.UUID) (Company, error) {
					return validCompany(uuid.New()), nil
				},
				update: func(context.Context, Company) error {
					return errors.Join(errors.New("update"), ErrNameConflict)
				},
			},
			wantErr: ErrNameConflict,
		},
	}

	for testName, test := range tests {
		test := test
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			service := NewService(test.repository, uuid.NewRandom)
			_, err := service.Patch(
				context.Background(),
				uuid.New(),
				PatchCompanyInput{Name: &name},
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Patch() error = %v, want wrapped %v", err, test.wantErr)
			}
		})
	}
}

func TestServiceDelete(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	deleteCalls := 0
	service := NewService(&fakeRepository{
		delete: func(_ context.Context, gotID uuid.UUID) error {
			deleteCalls++
			if gotID != id {
				t.Fatalf("Delete() ID = %s, want %s", gotID, id)
			}
			return nil
		},
	}, uuid.NewRandom)

	if err := service.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("Repository.Delete calls = %d, want 1", deleteCalls)
	}
}

func TestServiceDeletePropagatesNotFound(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{
		delete: func(context.Context, uuid.UUID) error {
			return errors.Join(errors.New("delete"), ErrNotFound)
		},
	}, uuid.NewRandom)

	err := service.Delete(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want wrapped ErrNotFound", err)
	}
}

type fakeRepository struct {
	create  func(context.Context, Company) error
	getByID func(context.Context, uuid.UUID) (Company, error)
	update  func(context.Context, Company) error
	delete  func(context.Context, uuid.UUID) error
}

func (repository *fakeRepository) Create(ctx context.Context, value Company) error {
	if repository.create == nil {
		return nil
	}
	return repository.create(ctx, value)
}

func (repository *fakeRepository) GetByID(ctx context.Context, id uuid.UUID) (Company, error) {
	if repository.getByID == nil {
		return Company{}, nil
	}
	return repository.getByID(ctx, id)
}

func (repository *fakeRepository) Update(ctx context.Context, value Company) error {
	if repository.update == nil {
		return nil
	}
	return repository.update(ctx, value)
}

func (repository *fakeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if repository.delete == nil {
		return nil
	}
	return repository.delete(ctx, id)
}

func validCreateInput() CreateCompanyInput {
	return CreateCompanyInput{
		Name:              "Acme",
		Description:       "Software",
		AmountOfEmployees: 10,
		Registered:        true,
		Type:              TypeCorporations,
	}
}

func validCompany(id uuid.UUID) Company {
	input := validCreateInput()
	return Company{
		ID:                id,
		Name:              input.Name,
		Description:       input.Description,
		AmountOfEmployees: input.AmountOfEmployees,
		Registered:        input.Registered,
		Type:              input.Type,
	}
}
