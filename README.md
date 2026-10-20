# Loan Service

A loan lifecycle service built with Go 1.27, Gin 1.11, GORM, and PostgreSQL. The API entry point is `cmd/server/main.go`; the notification worker entry point is `cmd/worker/main.go`.

The loan lifecycle is:

```text
PROPOSED → APPROVED → INVESTED → DISBURSED
```

## Getting Started

Edit `config/local.yml`, then start the dependencies and API from the project root:

```sh
docker compose up -d
go run ./cmd/server
```

Local endpoints:

- API: http://localhost:8080
- Mailpit inbox: http://localhost:8025
- PostgreSQL: `127.0.0.1:5433`, database `loans`

Compose starts PostgreSQL and Mailpit only. Run the API and worker with Go in separate terminals as shown below.

For a new database volume, Compose runs `001_init.up.sql` and `002_data_constraints.up.sql` in order. Existing volumes do not rerun these files. The app does not change tables at startup.

For an existing database that already has migration 001, apply migration 002 once:

```sh
docker compose exec -T db psql -U loans -d loans -v ON_ERROR_STOP=1 < migrations/002_data_constraints.up.sql
```

Migration 002 adds amount, rate, and state checks, plus one approval per loan. It keeps existing rows. If old data breaks a check or has duplicate approvals, the migration fails and rolls back. Review that data before retrying. The migration is not a repair script for older table layouts. Down migrations are for an intentional manual rollback only; they are never run at startup.

To run the API locally with PostgreSQL in Docker:

```sh
docker compose up -d db
go run ./cmd/server
```

In another terminal, start the notification worker:

```sh
go run ./cmd/worker
```

Both entry points read only `config/local.yml` by default. Run them from the project root. `.env` and environment variables do not override application settings.

## Configuration and Dependency Injection

Configuration uses a single YAML file, explicit constructors, and dependency injection. The API, worker, and integration tests share `config/local.yml`. Files are never merged and environment variables never override them.

```sh
go run ./cmd/server -conf config/local.yml
go run ./cmd/worker -conf config/local.yml
```

`config/local.yml` contains the shared local and integration-test settings. For a container deployment, mount the selected YAML at `/app/config/local.yml` or pass its path with `-conf`; database and SMTP addresses must be reachable from the container.

`config.NewConfig` loads and validates configuration. `repository.NewDB` creates the database connection. Google Wire assembles repositories, the service, handlers, and the HTTP server. `cmd/server/wire/wire.go` and `cmd/worker/wire/wire.go` declare provider sets; their `wire_gen.go` files are generated and checked in. Entry points call `wire.NewWire` and defer cleanup; the HTTP app handles OS signals and graceful shutdown. The HTTP constructor initializes tracing and returns its cleanup to Wire, which flushes tracing and closes the database pool after the app exits. Each database table has its own repository; the service coordinates transactions across them. After changing constructor dependencies, regenerate the wiring from the project root:

```sh
make wire
```

Normal builds use the generated files and do not run Wire.

`pkg/log.NewLog` creates the shared zap logger. `log.level` in the selected YAML controls the logging level.

## API

All business endpoints require `Authorization: Bearer <token>`. Health endpoints are public. The current authentication adapter maps configured tokens to trusted identities. Investor IDs, employee IDs, and investor email addresses are resolved from those identities.

| Method | Path | Role | Purpose |
|---|---|---|---|
| POST | `/api/v1/loans` | staff | Create a loan in `PROPOSED` status |
| POST | `/api/v1/loans/:loan_no/approval` | staff | Approve a loan and move it to `APPROVED` |
| POST | `/api/v1/loans/:loan_no/investments` | investor | Invest in an approved loan; full funding moves it to `INVESTED` |
| POST | `/api/v1/loans/:loan_no/disbursement` | staff | Disburse a fully funded loan and move it to `DISBURSED` |
| GET | `/api/v1/loans/:loan_no` | staff | Read loan details and current remaining amount |
| GET | `/healthz` | public | Process health |
| GET | `/readyz` | public | Database readiness |

Successful business responses use `{code,message,data}`, with business code `0` and HTTP status `200`. Creation returns a `loan_no`; use that business identifier in subsequent request paths. Internal numeric IDs are separate from `loan_no`.

Investment requests require an `Idempotency-Key` header of 1–255 characters. Repeating a key with the same loan number, investor ID, email, and amount returns the original stored response. Reusing a key with different request content returns HTTP `409` with business code `2003`. Keys are globally unique.

The checked-in tokens and database password are public local demo values. The investor email is `investor@example.com`. Use separate credentials and config for a deployed environment.

The examples below use tokens from `config/local.yml`. Replace them if you change `auth.tokens`. Every identity must have a non-empty `id`.

Create a loan:

```sh
curl --request POST 'http://localhost:8080/api/v1/loans' \
  --header 'Authorization: Bearer local-staff-token-change-123456' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "borrower_id": "BORROWER001",
    "principal_amount": 5000000,
    "rate": 10,
    "roi": 8,
    "agreement_url": "https://example.com/loan.pdf"
  }'
```

Set the returned loan number before making further requests:

```sh
LOAN_NO='<loan_no returned by the create response>'
```

Read the current loan:

```sh
curl "http://localhost:8080/api/v1/loans/${LOAN_NO}" \
  --header 'Authorization: Bearer local-staff-token-change-123456'
```

The result includes `loan_no`, `borrower_id`, `principal_amount`, `invested_amount`, `remaining_amount`, `rate`, `roi`, `agreement_url`, `status`, `created_at`, and `updated_at`. Rates in responses are decimal strings, such as `"10"`. Internal loan row IDs are hidden.

Approve the loan. The server sets the employee ID and approval time:

```sh
curl --request POST "http://localhost:8080/api/v1/loans/${LOAN_NO}/approval" \
  --header 'Authorization: Bearer local-staff-token-change-123456' \
  --header 'Content-Type: application/json' \
  --data-raw '{"visit_proof_url":"https://example.com/proof.jpg"}'
```

Invest in the approved loan:

```sh
curl --request POST "http://localhost:8080/api/v1/loans/${LOAN_NO}/investments" \
  --header 'Authorization: Bearer local-investor-token-change-123456' \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: investment-001' \
  --data-raw '{"amount":5000000}'
```

Use the same key when retrying an investment and a new key for each new investment.

Amounts are integers in the currency's smallest unit. Rates are percentages: `10` means 10%, with `0 <= roi <= rate <= 100`. The model stores rates as `NUMERIC(10,6)` decimals. This service does not transfer funds, accrue interest, or process repayments.

Disburse a fully funded loan. The server sets the employee ID and disbursement time:

```sh
curl --request POST "http://localhost:8080/api/v1/loans/${LOAN_NO}/disbursement" \
  --header 'Authorization: Bearer local-staff-token-change-123456' \
  --header 'Content-Type: application/json' \
  --data-raw '{"signed_agreement_url":"https://example.com/signed.pdf"}'
```

Approval and disbursement return `{"code":0,"message":"success","data":{}}`. Investment returns `id`, `loan_no`, `invested_amount`, `remaining_amount`, and `status` inside `data`.

Business timestamps use Unix milliseconds and come from the server. Client employee IDs, investor IDs, emails, and timestamps do not override auth or server values. Unknown JSON fields are ignored.

All document URLs must use HTTPS and have no embedded credentials:

| Field | Allowed path suffixes |
| --- | --- |
| Create: `agreement_url` | `.pdf` |
| Approve: `visit_proof_url` | `.jpg`, `.jpeg`, `.png` |
| Disburse: `signed_agreement_url` | `.pdf`, `.jpg`, `.jpeg` |

Suffix checks ignore letter case and query parameters. They do not prove the remote file's real content type. The service does not download files. File upload, agreement creation, and content checks belong to a separate file service.

The [OpenAPI specification](api/openapi.yaml) matches the current routes, request fields, response fields, and HTTP statuses.

## Error Responses

`api/v1.Error` defines separate HTTP statuses and business codes. Handlers call `HandleError(ctx, err)`. Wrapped errors retain their public mapping; unknown errors return HTTP `500` and business code `5000`, without exposing internal details.

Error responses contain only `code` and `message`. A state conflict returns HTTP `409`:

```json
{"code":2001,"message":"operation not allowed in current loan state"}
```

| Error | HTTP Status | Business Code |
|---|---:|---:|
| Invalid parameters | 400 | 1000 |
| Unauthenticated | 401 | 1001 |
| Forbidden | 403 | 1002 |
| Record not found | 404 | 1003 |
| Unsupported media type | 415 | 1004 |
| Invalid loan state | 409 | 2001 |
| Investment exceeds remaining principal | 409 | 2002 |
| Idempotency conflict | 409 | 2003 |
| Invalid approval time | 400 | 2004 |
| General conflict | 409 | 2005 |
| Internal error | 500 | 5000 |
| Database unavailable | 503 | 5001 |

An amount less than or equal to zero is an invalid parameter. A positive amount exceeding the current remaining principal is a business conflict.

## Testing

Ordinary tests use Go's `testing` package and `testify`; they do not require a database.

```sh
go test ./... -v
go test -race ./...
```

Database tests use the `integration` build tag and real PostgreSQL connections. Development and database tests share the `loans` database and account. Tests isolate their data in randomly named schemas and remove those schemas when finished.

Prepare PostgreSQL and verify `database.url` in `config/local.yml`:

```sh
docker compose up -d db
make integration

# Repeat PostgreSQL concurrency tests with race detection
go test -tags=integration -race -count=2 -timeout=120s ./tests/integration

# Run only the create-loan API test
go test -tags=integration ./tests/integration -run '^TestCreateLoan$' -v
```

Integration tests read only `config/local.yml`; they do not use environment variables. The API integration fixtures apply all `*.up.sql` migrations to their test schemas. Tests verify final database records for concurrent approval, funding, disbursement, idempotency conflicts, transaction rollback, and notification leases. Mail delivery uses a fake sender. Cleanup does not depend on an outer transaction rolling back repository transactions.

Existing integration fixtures may require updates when model fields or API signatures change. Run the ordinary tests and the relevant database tests after changing these contracts.

## Implementation and Scope

- `api/v1`: request and response types, public errors, and response handling.
- `internal/model`: database entities and status definitions, with one entity per file.
- `internal/repository`: one repository per table, responsible for that table's queries and writes.
- `internal/service`: business operations and transactions spanning multiple repositories.
- `internal/handler`: request binding, authenticated identities, and HTTP responses.
- `internal/server`: routes and middleware.
- `internal/notification`: Outbox processing, SMTP delivery, and retries.

Investment records, accumulated investment amounts, loan status changes, status history, and notification tasks are committed in one transaction. Investment transactions lock the loan row with PostgreSQL `FOR UPDATE` so concurrent requests for the same loan are serialized. Database uniqueness constraints protect idempotency keys. No process-local locks or Redis are required.

Database checks reject non-positive principal and investment amounts, negative invested amounts, over-investment, invalid states, and invalid rate/ROI values. A unique index allows only one approval per loan. The service still manages the investment sum across tables in one transaction.

When a loan becomes fully funded, notification recipients are deduplicated by investor and written to the Outbox in the same transaction. A separate worker sends the notifications. If the worker is stopped, tasks remain queued until it resumes.

The worker uses 60-second task leases and a 10-second send timeout. Failed sends are retried with a delay capped at ten minutes. No database transaction remains open during SMTP delivery. Delivery is at least once: a crash after sending but before acknowledging can cause a duplicate email. A stable Message-ID can help recipients deduplicate messages, but does not guarantee deduplication.

Configure SMTP through the `smtp` section in YAML (`address`, `from`, `username`, `password`). STARTTLS is required by default; `allow_plaintext: true` supports local Mailpit. Use trusted identities, appropriate database permissions, and TLS for deployed environments.

See [design.md](docs/design.md) for further design notes. Framework references: [Gin documentation](https://gin-gonic.com/en/docs/) and [GORM transactions](https://gorm.io/docs/transactions.html).

## Request Tracing and Logging

`pkg/tracer` initializes the global TracerProvider. `otelgin` creates HTTP spans and accepts W3C `traceparent` headers. Middleware adds tracing fields to the request context so downstream logs can share the same correlation identifiers.

Request logs include `request_id` and, when a valid span exists, `trace_id` and `span_id`. The response returns `X-Request-ID` and, when tracing is available, `X-Trace-ID`. A supplied `X-Request-ID` is reused; otherwise, one is generated. Startup and background error logs without a request context use a process correlation ID.

Tracing works locally without an external collector. Set `trace.endpoint: "http://localhost:4317"` in YAML to enable batched OTLP gRPC export. Use `http://` for a local plaintext collector and `https://` for TLS. `trace.name` defaults to `amartha-loan-service`. The API flushes and shuts down the TracerProvider on exit.

## Local Debugging in GoLand

```sh
docker compose up -d db
```

Open `cmd/server/main.go` in GoLand and click **Debug**. Set the working directory to the project root. The application reads `config/local.yml` by default; use program arguments `-conf <path>` to select another configuration file. Send requests to `http://localhost:8080`; the debugger's listening port is separate from the API port.
