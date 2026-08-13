# XM Company Service

A production-oriented Go service for creating, retrieving, partially updating, and deleting companies.

PostgreSQL persistence, JWT authentication, and production HTTP behavior are implemented. GET is public; POST, PATCH, and DELETE require a valid Bearer token.

## Architecture

```text
HTTP (JWT authentication for mutations)
  ↓
Company service
  ↓
Repository interface
  ↓
PostgreSQL
```

The service uses explicit constructor injection. HTTP, application behavior, and PostgreSQL persistence remain separate packages.

## Prerequisites

- Go 1.26
- Docker with Docker Compose
- OpenSSL for the sample local secret-generation command, or another source of a random 32-byte secret

Migration commands install the pinned `golang-migrate` CLI v4.19.1 into the ignored `.tools/` directory when needed.

## Configuration

The application reads actual environment variables:

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `DATABASE_URL` | Yes | — | PostgreSQL connection URL |
| `DB_MAX_CONNS` | No | `10` | Maximum PostgreSQL pool connections; must be a positive 32-bit integer |
| `HTTP_ADDR` | No | `:8080` | HTTP listen address |
| `LOG_LEVEL` | No | `INFO` | `DEBUG`, `INFO`, `WARN`, or `ERROR` |
| `LOG_FORMAT` | No | `text` | `text` or `json` |
| `JWT_SECRET` | Yes | — | HS256 shared secret of at least 32 bytes |
| `JWT_ISSUER` | Yes | — | Required token issuer |
| `JWT_AUDIENCE` | Yes | — | Audience that must be present in the token's `aud` claim |

[`.env.example`](.env.example) is documentation only and is not loaded automatically. Export its values in the shell or provide them through the process environment.

For local development:

```bash
export DATABASE_URL='postgres://company:company@localhost:5432/company_service?sslmode=disable'
export JWT_SECRET="$(openssl rand -hex 32)"
export JWT_ISSUER='xm-company-service-development'
export JWT_AUDIENCE='xm-company-service'
```

The service verifies tokens but never issues them. For local development, the separate `dev-token` command signs a short-lived token with the same configured values. Tokens from this tool and tokens from an external issuer follow exactly the same service authentication path. There is no development authentication mode or bypass in the service.

## Run locally

Start PostgreSQL and wait until it is healthy:

```bash
make db-up
```

Apply all pending migrations:

```bash
make migrate-up
```

Build or run the service:

```bash
make build
make run
```

Roll back one migration when needed:

```bash
make migrate-down
```

Stop PostgreSQL while preserving its named data volume:

```bash
make db-down
```

## API examples

Generate a development token after exporting the JWT configuration:

```bash
TOKEN=$(make -s dev-token)
```

The default lifetime is one hour. Override it with a duration of at least one second using whole-second precision:

```bash
TOKEN=$(make -s dev-token DEV_TOKEN_TTL=30m)
```

Create a company:

```bash
curl -i \
  -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Acme",
    "description": "Example company",
    "amount_of_employees": 10,
    "registered": true,
    "type": "Corporations"
  }' \
  http://localhost:8080/v1/companies
```

Retrieve the returned company ID:

```bash
curl -i http://localhost:8080/v1/companies/<uuid>
```

GET does not require authentication. PATCH and DELETE use the same Bearer header as POST:

```bash
curl -i \
  -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"amount_of_employees":20}' \
  http://localhost:8080/v1/companies/<uuid>

curl -i \
  -X DELETE \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/v1/companies/<uuid>
```

## Operational HTTP behavior

Liveness is independent of PostgreSQL. Readiness checks PostgreSQL with a one-second deadline:

```bash
curl -i http://localhost:8080/health/live
curl -i http://localhost:8080/health/ready
```

Both return `{"status":"ok"}` when healthy. Readiness returns the generic `503 SERVICE_UNAVAILABLE` error response when PostgreSQL cannot be reached.

Every application-handled HTTP response includes `X-Request-ID`. A valid, non-nil UUID supplied in exactly one `X-Request-ID` header is accepted and returned in canonical form; otherwise the service generates one. Structured completion logs include the same request ID, method, path, status, and duration without logging query strings or authorization values.

The HTTP server uses these fixed production limits:

| Setting | Value |
| --- | ---: |
| Header read timeout | 5s |
| Request read timeout | 10s |
| Application request deadline | 15s |
| Response write timeout | 20s |
| Idle connection timeout | 60s |
| Graceful shutdown period | 20s |

Application deadline expiration returns `503 SERVICE_UNAVAILABLE` when a response can still be written. Request contexts propagate through the service and repository to PostgreSQL, including while waiting for an available pooled connection.

The first `SIGINT` or `SIGTERM` restores default signal handling and begins bounded graceful shutdown. The service stops accepting new HTTP traffic, drains requests for up to 20 seconds, and force-closes HTTP if draining fails. PostgreSQL remains available until managed HTTP termination completes. A second termination signal uses the restored operating-system behavior and may end the process immediately without deferred cleanup.

## Tests

Run database-independent unit and handler tests:

```bash
make test
```

Start PostgreSQL, apply migrations, and run tagged repository and focused API integration tests:

```bash
make test-integration
```

Database-backed tests use `TEST_DATABASE_URL`; the Make target defaults it to the local Compose URL. Integration test data uses short unique names and each test removes only the rows it creates.

## Current assumptions

- Company name length is at most 15 Unicode characters.
- Description length is at most 3000 Unicode characters.
- Employee count cannot be negative.
- Company-name uniqueness is case-sensitive.
- UUIDs are generated by the service.
- GET-one is public.
- POST, PATCH, and DELETE require an HS256 JWT with the configured issuer, audience, and expiration.
- PATCH is a partial update.
- Description is non-null internally; JSON `null` is rejected.

RBAC, ownership checks, principal propagation, refresh tokens, identity-provider discovery, the application Docker image, and optional mutation events are deferred to later phases.
