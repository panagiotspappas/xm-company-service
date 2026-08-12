# XM Company Service

A production-oriented Go service for creating, retrieving, partially updating, and deleting companies.

## Status

The repository foundation is in place. Domain behavior, the HTTP API, PostgreSQL persistence, authentication, and operational concerns will be added as working vertical slices.

## Architecture

The service follows this dependency direction:

```text
HTTP
  ↓
Company service
  ↓
Repository interface
  ↓
PostgreSQL
```

Authentication supports the HTTP layer. Optional Kafka events are deferred until the mandatory functionality is complete.

## Prerequisites

- Go 1.26

## Development

Build the service:

```bash
make build
```

Run the current entry point:

```bash
make run
```

Run tests:

```bash
make test
```

Configuration, PostgreSQL, migrations, authentication, Docker, and integration-test instructions will be documented as those capabilities are implemented.
