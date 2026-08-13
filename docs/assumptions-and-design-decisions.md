# Assumptions and Design Decisions

This document distinguishes exercise decisions from local/evaluation conveniences and broader production deployment concerns.

## Authentication boundary

The service is a resource server. It validates Bearer JWTs for POST, PATCH, and DELETE, while company GET and health endpoints remain public. It does not implement login, users, passwords, refresh tokens, or identity management.

The self-contained exercise uses one externally supplied HS256 secret, exact issuer validation, presence of the configured audience, and a required non-expired `exp` claim. All verification failures are normalized to the same `401 UNAUTHORIZED` response instead of exposing JWT-library details.

HS256 keeps this exercise locally runnable. A larger multi-service deployment would commonly delegate issuance to an identity provider and validate asymmetric signatures through published keys/JWKS rather than share one HMAC secret among services.

## TLS and PostgreSQL transport

The Go process does not terminate TLS. The deployment boundary assumes HTTPS terminates in infrastructure such as an ingress, reverse proxy, load balancer, or API gateway before traffic reaches the service. This is not a recommendation to expose unencrypted public traffic.

Local and Compose PostgreSQL URLs use `sslmode=disable` only for local/evaluation topologies. With native `make run`, the service reaches the Dockerized PostgreSQL instance through `localhost`; with the containerized stack, it uses the private Compose network. A production DATABASE_URL can enable PostgreSQL TLS according to the database provider's requirements. Because the URL may contain database credentials, it is treated as secret configuration. For that reason, this service keeps DATABASE_URL environment-only rather than allowing it in the JSON configuration file. In production, it should ideally be supplied through the deployment environment or a secret-management system rather than committed to configuration files.

## Synchronous request handling and backpressure

CRUD operations execute synchronously on the request path. Go's HTTP server already handles concurrent requests with goroutines; there is no asynchronous job, CPU-intensive queue, or batch workload that needs an application worker pool. Adding one would introduce queueing, scheduling, cancellation, and shutdown complexity without satisfying a requirement.

Database concurrency was already bounded by pgxpool's default maximum-connection behavior. This implementation makes that boundary explicit and deployment-controllable through `DB_MAX_CONNS`, which defaults to 10:

	concurrent HTTP requests
			↓
	request-scoped service/repository work
			↓
	bounded pgx connection pool
			↓
	PostgreSQL

When the pool is saturated, callers wait for an available connection rather than causing the application to create unlimited database connections. Request deadlines and cancellation propagate through pool acquisition and database operations.

Making the connection limit explicit avoids relying on a library/runtime-derived default and allows the deployment to tune database concurrency according to PostgreSQL capacity and the number of running service instances.

## Company model decisions

### Partial updates

PATCH was selected instead of PUT because clients update only supplied fields; omitted fields retain their stored values. Explicit zero values, `false`, and an empty description remain meaningful updates.

### Server-owned identity

The service generates company UUIDs. Callers do not choose persistent identifiers.

### Name uniqueness

Names are unique with case-sensitive semantics. PostgreSQL's unique constraint is the authoritative concurrency-safe enforcement point. The repository translates that specific constraint violation into the domain conflict used by the HTTP `409` response.

### Unicode lengths

The 15-character name and 3000-character description limits count Unicode runes rather than UTF-8 bytes, so multibyte characters are not counted several times.

### Description representation

Description is a non-null string in the domain and database. No description is represented as `""`, avoiding a second nullable state. On create, omission becomes empty; on PATCH, omission preserves the current value and `""` clears it. JSON `null` is rejected.

## Operational HTTP behavior

### Request correlation

Every application-handled request receives an `X-Request-ID`. A single valid, non-nil UUID from the client is canonicalized and reused; otherwise one is generated. Structured completion logs carry the same ID so activity for one request can be correlated. This is request correlation, not complete distributed tracing.

Completion logs contain method, URL path without query data, status, duration, and request ID. Bodies, query strings, Authorization headers, JWTs, and secrets are intentionally excluded.

### Health checks

Liveness answers whether the application process is alive and intentionally does not depend on PostgreSQL. Readiness checks whether PostgreSQL is reachable with a one-second bounded context. This lets infrastructure distinguish a running process from one that cannot currently serve dependency-backed work.

### Timeouts

The server uses fixed operational limits:

| Setting | Value |
| --- | ---: |
| Header read timeout | 5s |
| Request read timeout | 10s |
| Application request deadline | 15s |
| Response write timeout | 20s |
| Idle connection timeout | 60s |
| Readiness deadline | 1s |

These bounds prevent stalled clients or dependencies from occupying resources indefinitely while preserving request-context cancellation through the service and database layers.

### Graceful shutdown

The first SIGINT or SIGTERM stops signal interception and begins a bounded 20-second HTTP shutdown: stop accepting new work, allow in-flight requests to finish, force-close HTTP if necessary, and close PostgreSQL only after managed HTTP termination. Restoring default signal behavior lets a second signal terminate immediately.

Compose uses a 30-second stop grace, leaving time for the application's 20-second shutdown path before container-level termination.

## Packaging and lifecycle

### Explicit migrations

Schema changes live in versioned migration files rather than application-startup schema mutation. Compose orders startup as:

```text
PostgreSQL healthy
→ one-shot migration succeeds
→ company-service starts
```

### No Compose restart policy

The supplied Compose model targets development, evaluation, integration tests, and acceptance runs. Automatically restarting a crashed service could hide a failure during those workflows. A production orchestrator would normally own restart/rescheduling policy.

### Configuration and secrets

For settings supported by the optional strict JSON file, precedence is a nonblank file value, otherwise a nonblank environment value, otherwise the built-in default. Blank or whitespace-only JSON string values are treated as absent. The JSON file is only for non-secret runtime settings. `JWT_SECRET` remains environment-only, and `DATABASE_URL` remains environment-only because it may include credentials. The production image contains neither configuration files nor secrets.

## Deferred capabilities

The current service deliberately does not add RBAC, ownership rules, principal propagation, refresh tokens, identity-provider discovery, distributed tracing, additional metrics instrumentation, rate limiting, Kafka/events, an outbox, Kubernetes manifests, Helm charts, or in-process TLS termination. These require separate operational or product requirements and should not be inferred from the exercise implementation.
