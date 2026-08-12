.PHONY: build run test dev-token db-up db-down migrate-up migrate-down test-integration

BIN_DIR := bin
BINARY := $(BIN_DIR)/company-service
TOOLS_BIN := $(CURDIR)/.tools/bin
MIGRATE := $(TOOLS_BIN)/migrate
MIGRATE_VERSION := v4.19.1
LOCAL_DATABASE_URL := postgres://company:company@localhost:5432/company_service?sslmode=disable
DATABASE_URL ?= $(LOCAL_DATABASE_URL)
TEST_DATABASE_URL ?= $(LOCAL_DATABASE_URL)
DEV_TOKEN_TTL ?= 1h

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/company-service

run:
	go run ./cmd/company-service

test:
	go test ./...

dev-token:
	@go run ./cmd/dev-token -ttl "$(DEV_TOKEN_TTL)"

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

$(MIGRATE):
	mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

migrate-up: $(MIGRATE)
	@$(MIGRATE) -path migrations -database "$(DATABASE_URL)" up

migrate-down: $(MIGRATE)
	@$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down 1

test-integration: db-up $(MIGRATE)
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" $(MIGRATE) -path migrations -database "$(TEST_DATABASE_URL)" up
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration -count=1 ./internal/postgres ./integration
