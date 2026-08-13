.DEFAULT_GOAL := help

.PHONY: help build run test vet lint check dev-token db-up db-down stack-up stack-down stack-logs migrate-up migrate-down test-integration

BIN_DIR := bin
BINARY := $(BIN_DIR)/company-service
TOOLS_BIN := $(CURDIR)/.tools/bin
MIGRATE := $(TOOLS_BIN)/migrate
MIGRATE_VERSION := v4.19.1
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint
GOLANGCI_LINT_VERSION := v2.12.2
LOCAL_DATABASE_URL := postgres://company:company@localhost:5432/company_service?sslmode=disable
DATABASE_URL ?= $(LOCAL_DATABASE_URL)
TEST_DATABASE_URL ?= $(LOCAL_DATABASE_URL)
DEV_TOKEN_TTL ?= 1h

help:
	@printf '%s\n' \
		'Available commands:' \
		'  make build              Build the company-service binary' \
		'  make run                Run the company service' \
		'  make test               Run unit tests' \
		'  make vet                Run go vet' \
		'  make lint               Run the pinned linters' \
		'  make check              Run test, vet, lint, and build' \
		'  make dev-token          Generate a development JWT' \
		'  make db-up              Start PostgreSQL' \
		'  make db-down            Stop and remove PostgreSQL only' \
		'  make stack-up           Build and start the complete Compose stack' \
		'  make stack-down         Stop the Compose stack' \
		'  make stack-logs         Follow company-service logs' \
		'  make migrate-up         Apply database migrations' \
		'  make migrate-down       Roll back one migration' \
		'  make test-integration   Run PostgreSQL integration tests'

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/company-service

run:
	go run ./cmd/company-service

test:
	go test ./...

vet:
	go vet ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

check: test vet lint build

dev-token:
	@go run ./cmd/dev-token -ttl "$(DEV_TOKEN_TTL)"

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose rm --stop --force postgres

stack-up:
	@if [ -z "$$(printf '%s' "$${CONFIG_FILE:-}" | tr -d '[:space:]')" ]; then \
		docker compose up -d --build --wait; \
	else \
		config_file="$$CONFIG_FILE"; \
		case "$$config_file" in \
			/*) ;; \
			*) config_file="$(CURDIR)/$$config_file" ;; \
		esac; \
		if [ ! -f "$$config_file" ]; then \
			printf 'CONFIG_FILE is not a regular file: %s\n' "$$config_file" >&2; \
			exit 1; \
		fi; \
		CONFIG_FILE="$$config_file" docker compose \
			-f compose.yaml \
			-f compose.config-file.yaml \
			up -d --build --wait; \
	fi

stack-down:
	docker compose down

stack-logs:
	docker compose logs -f company-service

$(MIGRATE):
	mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

$(GOLANGCI_LINT):
	mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

migrate-up: $(MIGRATE)
	@$(MIGRATE) -path migrations -database "$(DATABASE_URL)" up

migrate-down: $(MIGRATE)
	@$(MIGRATE) -path migrations -database "$(DATABASE_URL)" down 1

test-integration: db-up $(MIGRATE)
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" $(MIGRATE) -path migrations -database "$(TEST_DATABASE_URL)" up
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration -count=1 ./internal/postgres ./integration
