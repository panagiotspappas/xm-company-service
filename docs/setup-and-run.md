# Setup and Run Instructions

This guide takes a fresh checkout from prerequisites to a running API. Run commands from the repository root.

## Prerequisites

For the recommended Docker workflow:

- Docker with Docker Compose
- Make
- curl

Local execution, development-token generation, tests, and lint installation also require the Go version declared in `go.mod` (currently Go 1.26).

The Makefile installs pinned development tools into the ignored `.tools/bin` directory when required:

- `golang-migrate` v4.19.1
- `golangci-lint` v2.12.2

## Recommended: run the complete stack

Export one JWT configuration shared by the container and the host-side development-token command:

```bash
export JWT_SECRET='at-least-32-byte-secret-value-1234'
export JWT_ISSUER='xm-company-service'
export JWT_AUDIENCE='xm-company-service'
```

The example secret is suitable only for local evaluation. Supply a securely generated secret in a real deployment.

Build and start the complete environment:

```bash
make stack-up
```

The command waits for this dependency chain:

```text
PostgreSQL healthy
→ migrations complete successfully
→ company-service starts
→ readiness succeeds
```

Verify the public health endpoints:

```bash
curl -i http://localhost:8080/health/live
curl -i http://localhost:8080/health/ready
docker compose ps -a
```

With the complete stack healthy, both endpoints return `200 OK` with `{"status":"ok"}`. Liveness does not depend on PostgreSQL; readiness does.

The migration container should show a successful exit, while PostgreSQL and the application should be healthy.

### Generate a development token

### Generate a development token

Generate a one-hour HS256 token using the exported secret, issuer, and audience:

```bash
TOKEN=$(make -s dev-token)
```

The development token lifetime defaults to one hour.

To use a different lifetime, optionally set `DEV_TOKEN_TTL`. The value uses Go duration syntax, must be positive, and must resolve to a whole number of seconds. For example:

```bash
TOKEN=$(make -s dev-token DEV_TOKEN_TTL=30m)
```

The command prints only the token to standard output. It does not enable a special development authentication mode; requests continue through the normal authentication middleware and JWT validator.

### Exercise the API

Create a company:

```bash
curl -i \
  -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Alpha Tech",
    "description": "Software company",
    "amount_of_employees": 12,
    "registered": true,
    "type": "Corporations"
  }' \
  http://localhost:8080/v1/companies
```

Copy the `id` from the JSON response and export it:

```bash
export COMPANY_ID='<returned-company-uuid>'
```

GET is public:

```bash
curl -i "http://localhost:8080/v1/companies/$COMPANY_ID"
```

Partially update the company:

```bash
curl -i \
  -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"amount_of_employees":20,"registered":false}' \
  "http://localhost:8080/v1/companies/$COMPANY_ID"
```

Delete it:

```bash
curl -i \
  -X DELETE \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/v1/companies/$COMPANY_ID"
```

See the [API Contract](api-contract.md) for the complete request and error behavior.

### Logs and shutdown

Follow application logs interactively:

```bash
make stack-logs
```

This command blocks until interrupted.

Stop the complete stack with:

```bash
make stack-down
```

The named PostgreSQL volume is preserved.

## PostgreSQL-only local development

This mode runs PostgreSQL in Docker while running the Go service directly on the host.

Export the application configuration:

```bash
export DATABASE_URL='postgres://company:company@localhost:5432/company_service?sslmode=disable'
export JWT_SECRET='at-least-32-byte-secret-value-1234'
export JWT_ISSUER='xm-company-service'
export JWT_AUDIENCE='xm-company-service'
```

Start PostgreSQL, apply the migrations, and run the Go service:

```bash
make db-up
make migrate-up
make run
```

`make run` stays in the foreground and serves the same API at:

```text
http://localhost:8080
```

`make run` stays in the foreground and serves the API at:

```text
http://localhost:8080
```

Leave this terminal running.

Open a second terminal in the repository root and export the same JWT configuration used by the running service:

```bash
export JWT_SECRET='at-least-32-byte-secret-value-1234'
export JWT_ISSUER='xm-company-service'
export JWT_AUDIENCE='xm-company-service'
```

Generate a development token:

```bash
TOKEN=$(make -s dev-token)
```

You can then use the same GET, POST, PATCH, and DELETE commands shown in the Docker walkthrough above.

When finished, return to the terminal running `make run` and stop the service with `Ctrl+C`.

Then stop and remove only the PostgreSQL container:

```bash
make db-down
```

The PostgreSQL named volume is preserved. `db-down` does not intentionally tear down the full Compose project.

## Configuration

Configuration precedence is:

```text
built-in defaults
→ optional CONFIG_FILE
→ nonblank environment variables
```

| Environment variable | Required | Default | Source rules |
| --- | --- | --- | --- |
| `CONFIG_FILE` | No | none | Optional path to strict JSON configuration |
| `HTTP_ADDR` | No | `:8080` | Environment or JSON file |
| `DATABASE_URL` | Yes | none | Environment only; may contain credentials |
| `DB_MAX_CONNS` | No | `10` | Environment or JSON file; positive 32-bit integer |
| `LOG_LEVEL` | No | `INFO` | `DEBUG`, `INFO`, `WARN`, or `ERROR` |
| `LOG_FORMAT` | No | `text` | `text` or `json` |
| `JWT_SECRET` | Yes | none | Environment only; raw value must be at least 32 bytes |
| `JWT_ISSUER` | Yes | none | Environment or JSON file |
| `JWT_AUDIENCE` | Yes | none | Environment or JSON file |

`.env.example` documents environment variables but is not loaded automatically.

`config.example.json` demonstrates the supported non-secret JSON fields:

- `http_addr`
- `db_max_conns`
- `log_level`
- `log_format`
- `jwt_issuer`
- `jwt_audience`

The JSON decoder rejects unreadable files, malformed or trailing JSON, unknown fields, explicit `null`, invalid types, and invalid final values.

`database_url` and `jwt_secret` are intentionally unsupported in the file.

### Optional: use a configuration file with Docker

The normal Docker workflow above does not require `CONFIG_FILE`. The production image does not contain `config.example.json`.

To use file-backed configuration, mount the chosen configuration file into the container, preferably read-only, and set `CONFIG_FILE` to its path inside the container.

The base Compose configuration supplies non-secret environment defaults. Non-blank environment values take precedence over values from the configuration file. Therefore, to source those settings from the mounted JSON file instead, create a local Compose override such as `compose.config-file.yaml`:

```yaml
services:
  company-service:
    volumes:
      - ./config.example.json:/app/config.json:ro
    environment:
      CONFIG_FILE: /app/config.json
      HTTP_ADDR: ""
      DB_MAX_CONNS: ""
      LOG_LEVEL: ""
      LOG_FORMAT: ""
      JWT_ISSUER: ""
      JWT_AUDIENCE: ""
```

The empty environment values above allow the corresponding values from the JSON configuration file to be used.

Then start Compose with both files while continuing to supply the secret through the environment:

```bash
export JWT_SECRET='at-least-32-byte-secret-value-1234'
COMPOSE_FILE=compose.yaml:compose.config-file.yaml make stack-up
```

The base Compose configuration continues to provide the internal `DATABASE_URL`. `DATABASE_URL` and `JWT_SECRET` remain environment-only and are not part of the JSON configuration schema.

If you use the host-side development-token command with this configuration, ensure its issuer and audience match the values in the mounted JSON file. For example, if the file contains the standard example values:

```bash
export JWT_ISSUER='xm-company-service'
export JWT_AUDIENCE='xm-company-service'
TOKEN=$(make -s dev-token)
```

Do not bake configuration files or secrets into the production image.

## Verification commands

| Command | Behavior |
| --- | --- |
| `make test` | Database-independent unit and handler tests |
| `make vet` | `go vet ./...` |
| `make lint` | Pinned conservative linters, including integration-tagged code |
| `make build` | Build `bin/company-service` |
| `make check` | Test, vet, lint, and build; no database required |
| `make test-integration` | Start real PostgreSQL, migrate it, and run repository/API integration suites |

## Health and troubleshooting

`GET /health/live` reports whether the application process is alive and does not query PostgreSQL.

`GET /health/ready` checks PostgreSQL with a one-second deadline and returns `503 SERVICE_UNAVAILABLE` when the dependency is unavailable.

Common problems:

- **JWT configuration rejected:** ensure `JWT_SECRET` is at least 32 bytes and the issuer/audience are nonblank.
- **Token returns 401:** generate it with the same secret, issuer, and audience used by the service, and ensure it has not expired.
- **Port 8080 is occupied:** stop the conflicting process or container before `make stack-up`.
- **Application is live but not ready:** inspect PostgreSQL with `docker compose ps -a` and application logs with `make stack-logs`.
- **PostgreSQL unavailable locally:** run `make db-up` and `make migrate-up` before `make run`.
- **Docker command unavailable:** ensure Docker is running and Docker Compose/WSL integration is enabled for the current environment.