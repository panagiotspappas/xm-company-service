# API Contract

The API uses JSON over HTTP. Company mutations require a valid Bearer JWT; company retrieval and health endpoints are public.

## Routes and authentication

| Method and path | Authentication |
| --- | --- |
| `POST /v1/companies` | Bearer JWT required |
| `GET /v1/companies/{id}` | Public |
| `PATCH /v1/companies/{id}` | Bearer JWT required |
| `DELETE /v1/companies/{id}` | Bearer JWT required |
| `GET /health/live` | Public |
| `GET /health/ready` | Public |

Authentication is evaluated before request parsing on protected routes. A missing, duplicate, malformed, invalid, or expired Bearer credential returns `401 Unauthorized` with `WWW-Authenticate: Bearer`.

## Company representation

```json
{
  "id": "a09c4dbf-c1f9-49af-bfd0-269fd98247b6",
  "name": "Alpha Tech",
  "description": "Software company",
  "amount_of_employees": 12,
  "registered": true,
  "type": "Corporations"
}
```

Allowed `type` values are exact and case-sensitive:

- `Corporations`
- `NonProfit`
- `Cooperative`
- `Sole Proprietorship`

The service generates the UUID. Route IDs must parse as UUIDs.

## Validation semantics

| Field | Create | Patch and validation behavior |
| --- | --- | --- |
| `name` | Required | Omitted means unchanged. A supplied value replaces the name. Whitespace-only values are rejected; maximum 15 Unicode runes. The submitted string is otherwise preserved. |
| `description` | Optional; omission becomes `""` | Omitted means unchanged; `""` clears it. Maximum 3000 Unicode runes. Explicit `null` is rejected. |
| `amount_of_employees` | Required integer; `0` is valid | Omitted means unchanged; a supplied integer replaces it. Negative values are rejected. |
| `registered` | Required boolean; `false` is valid | Omitted means unchanged; a supplied boolean replaces it. |
| `type` | Required allowed value | Omitted means unchanged; a supplied value must exactly match an allowed type. |

An empty PATCH object is rejected. Company-name uniqueness is case-sensitive: `Acme` and `ACME` are distinct. PostgreSQL's unique constraint is authoritative, including under concurrent requests.

## Transport rules

POST and PATCH require a parseable `Content-Type` whose media type is `application/json`. Parameters such as `application/json; charset=utf-8` are accepted. A missing, malformed, or different media type returns `415 Unsupported Media Type`.

POST and PATCH bodies have a fixed 1 MiB limit. Oversized bodies return `413 Request Entity Too Large`.

Strict decoding rejects with `400 Invalid Request`:

- Empty request bodies
- Malformed JSON
- Unknown fields
- A top-level `null`
- Explicit `null` fields
- Wrong JSON field types
- Additional/trailing JSON values
- Missing required create fields
- An empty PATCH object
- Domain-validation failures

Handler-generated JSON responses use `Content-Type: application/json`. Native `http.ServeMux` responses for an unmatched route or method mismatch may be plain text rather than the JSON error envelope.

Production middleware adds `X-Request-ID` to application-handled responses. It accepts and canonicalizes a single valid, non-nil UUID supplied by the client; otherwise it generates one.

## Endpoints

### POST `/v1/companies`

Authentication: required.

```json
{
  "name": "Alpha Tech",
  "description": "Software company",
  "amount_of_employees": 12,
  "registered": true,
  "type": "Corporations"
}
```

Success:

- `201 Created`
- `Content-Type: application/json`
- `Location: /v1/companies/{generated-id}`
- Complete company representation in the response body

Relevant errors: `400`, `401`, `409`, `413`, `415`, `500`, and `503`.

### GET `/v1/companies/{id}`

Authentication: public.

Success:

- `200 OK`
- `Content-Type: application/json`
- Complete company representation

An invalid UUID returns `400 INVALID_REQUEST`; an unknown valid UUID returns `404 COMPANY_NOT_FOUND`.

### PATCH `/v1/companies/{id}`

Authentication: required.

Send one or more company fields. For example:

```json
{
  "description": "Updated description",
  "amount_of_employees": 0,
  "registered": false
}
```

Success:

- `200 OK`
- `Content-Type: application/json`
- Complete updated company representation

Relevant errors: `400`, `401`, `404`, `409`, `413`, `415`, `500`, and `503`.

### DELETE `/v1/companies/{id}`

Authentication: required.

Success is `204 No Content` with an empty body and no `Content-Type` header added by the handler. An invalid UUID returns `400`; a missing company returns `404`.

### GET `/health/live`

Authentication: public.

Returns `200 OK` independently of PostgreSQL:

```json
{"status":"ok"}
```

The response uses `Content-Type: application/json`.

### GET `/health/ready`

Authentication: public.

Returns the same `200 OK` JSON body and content type when PostgreSQL responds within the one-second readiness deadline. A PostgreSQL error or readiness timeout returns `503 SERVICE_UNAVAILABLE` without exposing database details.

## Error contract

Handler-generated errors use this envelope:

```json
{
  "error": {
    "code": "INVALID_REQUEST",
    "message": "invalid request"
  }
}
```

| Status | Code | Message | Meaning |
| ---: | --- | --- | --- |
| `400` | `INVALID_REQUEST` | `invalid request` | Invalid UUID, JSON, required fields, PATCH shape, or company validation |
| `401` | `UNAUTHORIZED` | `unauthorized` | Missing, malformed, invalid, or expired Bearer token |
| `404` | `COMPANY_NOT_FOUND` | `company not found` | No company exists for the valid UUID |
| `409` | `COMPANY_NAME_CONFLICT` | `company name conflict` | The exact case-sensitive name already exists |
| `413` | `CONTENT_TOO_LARGE` | `request body is too large` | POST/PATCH body exceeded 1 MiB |
| `415` | `UNSUPPORTED_MEDIA_TYPE` | `unsupported media type` | POST/PATCH did not provide supported JSON media type |
| `500` | `INTERNAL_ERROR` | `internal server error` | Unexpected service/repository failure or a recoverable pre-response panic |
| `503` | `SERVICE_UNAVAILABLE` | `service unavailable` | PostgreSQL readiness failure or application request-deadline expiration when a response can still be written |

JWT-library errors, PostgreSQL errors, and panic details are not exposed to clients.
