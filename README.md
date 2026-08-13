# XM Company Service

A production-oriented Go implementation of the XM company-management exercise. It provides a REST API backed by PostgreSQL,
JWT-protected mutations, explicit database migrations, production HTTP safeguards, Docker packaging, integration tests,
conservative linting, and GitHub Actions CI.

## Getting Started

For complete fresh-clone setup, configuration, JWT generation, and API examples,
see the [Setup and Run Instructions](docs/setup-and-run.md).

The shortest Docker-based start is:

```bash
export JWT_SECRET='at-least-32-byte-secret-value-1234'
make stack-up
curl -i http://localhost:8080/health/ready
```

`make stack-up` builds the production image, starts PostgreSQL, applies migrations, starts the application, and waits for readiness.

## API

| Method and path | Authentication |
| --- | --- |
| `POST /v1/companies` | Bearer JWT required |
| `GET /v1/companies/{id}` | Public |
| `PATCH /v1/companies/{id}` | Bearer JWT required |
| `DELETE /v1/companies/{id}` | Bearer JWT required |
| `GET /health/live` | Public |
| `GET /health/ready` | Public |

The service verifies tokens but does not issue them or implement login. See the [Setup and Run Instructions](docs/setup-and-run.md) for development-token generation and authenticated API examples, and the [API Contract](docs/api-contract.md) for payloads, validation, statuses, and errors.

## Commands

| Command | Purpose |
| --- | --- |
| `make db-up` | Start PostgreSQL only |
| `make db-down` | Stop/remove PostgreSQL only, preserving its volume |
| `make stack-up` | Build and start PostgreSQL, migrations, and the application |
| `make stack-down` | Stop the complete Compose stack, preserving its volume |
| `make stack-logs` | Interactively follow `company-service` logs |
| `make test` | Run database-independent tests |
| `make vet` | Run `go vet` |
| `make lint` | Run the pinned conservative linter set |
| `make build` | Build the service binary |
| `make check` | Run tests, vet, lint, and build |
| `make test-integration` | Run repository and API suites against real PostgreSQL |

Integration tests remain separate from `make check` because they require Docker and PostgreSQL.

## Architecture

```text
HTTP API and JWT boundary
          ↓
Company service
          ↓
Repository interface
          ↓
PostgreSQL
```

Request IDs, structured completion logging, panic recovery, bounded request/server timeouts, health endpoints, graceful shutdown, and a bounded PostgreSQL connection pool are active in the production server.

GitHub Actions runs `make check`, real PostgreSQL integration tests, a production Docker image build, and Compose configuration validation.

## Documentation

- [Setup and Run Instructions](docs/setup-and-run.md)
- [API Contract](docs/api-contract.md)
- [Assumptions and Design Decisions](docs/assumptions-and-design-decisions.md)
