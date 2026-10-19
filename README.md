# Loan Service

A loan lifecycle service built with Go 1.27, Gin 1.11, GORM, and PostgreSQL. The API entry point is `cmd/server/main.go`; the notification worker entry point is `cmd/worker/main.go`.

The loan lifecycle is:

```text
PROPOSED → APPROVED → INVESTED → DISBURSED
```

## Getting Started

```sh
cp .env.example .env
# Edit .env and replace the example authentication tokens.
# Docker Compose reads .env automatically.
docker compose up --build -d
```

Local endpoints:

- API: http://localhost:8080
- Mailpit inbox: http://localhost:8025
- PostgreSQL: `127.0.0.1:5433`, database `loans`

When the PostgreSQL data volume is first initialized, Compose runs the mounted SQL migrations in order: `001_init.up.sql`, `002_loan_rate_columns.up.sql`, and `003_decimal_rates.up.sql`. Existing volumes do not rerun these scripts. Apply database changes through migrations; the application does not automatically update the database schema.

To run the API locally with PostgreSQL in Docker:

```sh
docker compose up -d db
# Stop the containerized API to free port 8080.
docker compose stop api
go run ./cmd/server
```

In another terminal, start the notification worker:

```sh
go run ./cmd/worker
```

Both entry points automatically load `.env` from the working directory or project root. Existing process environment variables take precedence.

## Configuration and Dependency Injection

Configuration uses Viper, explicit constructors, and dependency injection. Use `-conf` to load a YAML configuration file. `APP_CONF` overrides the command-line configuration path, and environment variables override YAML values.

```sh
go run ./cmd/server -conf config/local.yml
go run ./cmd/worker -conf config/local.yml
```

An environment-only deployment does not require a YAML file. See [.env.example](.env.example) for the available settings.

`config.NewConfig` loads and validates configuration. `repository.NewDB` creates the database connection. The entry point assembles repositories, the service, handlers, and the HTTP server. Each database table has its own repository; the service coordinates transactions across them.

`pkg/log.NewLog` creates the shared zap logger. `LOG_LEVEL` controls the logging level.

## API

All business endpoints require `Authorization: Bearer <token>`. Health endpoints are public. The current authentication adapter maps configured tokens to trusted identities. Investor IDs, employee IDs, and investor email addresses are resolved from those identities.

| Method | Path | Role | Purpose |
|---|---|---|---|
| POST | `/api/v1/loans` | staff | Create a loan in `PROPOSED` status |
| POST | `/api/v1/loans/:loan_no/approval` | staff | Approve a loan and move it to `APPROVED` |
| POST | `/api/v1/loans/:loan_no/investments` | investor | Invest in an approved loan; full funding moves it to `INVESTED` |
| POST | `/api/v1/loans/:loan_no/disbursement` | staff | Disburse a fully funded loan and move it to `DISBURSED` |
| GET | `/health` | public | Process health |
| GET | `/healthz` | public | Process health |
| GET | `/readyz` | public | Database readiness |

Successful business responses use `{code,message,data}`, with business code `0` and HTTP status `200`. Creation returns a `loan_no`; use that business identifier in subsequent request paths. Internal numeric IDs are separate from `loan_no`.

Investment requests require an `Idempotency-Key` header of 1–255 characters. Repeating a key with the same loan number, email, and amount returns the original stored response. Reusing a key with different request content returns HTTP `409` with business code `2003`. Keys are globally unique.

The examples below use tokens from `.env.example`. Replace them with the corresponding staff or investor token from your `.env` if you have changed the configuration.

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

Approve the loan using the current Unix timestamp in milliseconds:

```sh
curl --request POST "http://localhost:8080/api/v1/loans/${LOAN_NO}/approval" \
  --header 'Authorization: Bearer local-staff-token-change-123456' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"visit_proof_url\": \"https://example.com/proof.jpg\",
    \"approved_at\": $(python3 -c 'import time; print(time.time_ns() // 1000000)')
  }"
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

Business timestamps use Unix milliseconds. Approval cannot precede loan creation. URLs represent externally managed documents; the service stores and sends links rather than downloading files. File upload, agreement generation, and verification of actual file content belong to a separate file service.

An [OpenAPI specification](api/openapi.yaml) is included. Some sections still reflect earlier API revisions; the route table and examples above describe the current handlers.

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

Prepare the local database:

```sh
# Reuse the existing container and data volume.
docker compose up -d db
# Create .env only if it does not already exist.
cp .env.example .env
```

The Makefile reads `.env` and exports `DATABASE_URL`. Direct `go test` commands do not automatically load `.env`:

```sh
set -a
. ./.env
set +a

# Service-level investment tests
go test -tags=integration -race -count=1 ./internal/service

# API integration tests
make integration

# Run only the create-loan API test
go test -tags=integration ./tests/integration -run '^TestCreateLoan$' -v
```

Integration tests require `DATABASE_URL`. The API integration fixtures apply all `*.up.sql` migrations to their test schemas. Service investment fixtures create isolated tables for their model contracts. Cleanup does not depend on an outer transaction rolling back repository transactions.

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

When a loan becomes fully funded, notification recipients are deduplicated by investor and written to the Outbox in the same transaction. A separate worker sends the notifications. If the worker is stopped, tasks remain queued until it resumes.

The worker uses 60-second task leases and a 10-second send timeout. Failed sends are retried with a delay capped at ten minutes. No database transaction remains open during SMTP delivery. Delivery is at least once: a crash after sending but before acknowledging can cause a duplicate email. A stable Message-ID can help recipients deduplicate messages, but does not guarantee deduplication.

Configure SMTP through `SMTP_ADDR`, `SMTP_FROM`, `SMTP_USERNAME`, and `SMTP_PASSWORD`. STARTTLS is required by default; `SMTP_ALLOW_PLAINTEXT=true` supports local Mailpit. Use trusted identities, appropriate database permissions, and TLS for deployed environments.

See [design.md](docs/design.md) for further design notes. Framework references: [Gin documentation](https://gin-gonic.com/en/docs/) and [GORM transactions](https://gorm.io/docs/transactions.html).

## Request Tracing and Logging

`pkg/tracer` initializes the global TracerProvider. `otelgin` creates HTTP spans and accepts W3C `traceparent` headers. Middleware adds tracing fields to the request context so downstream logs can share the same correlation identifiers.

Request logs include `request_id` and, when a valid span exists, `trace_id` and `span_id`. The response returns `X-Request-ID` and, when tracing is available, `X-Trace-ID`. A supplied `X-Request-ID` is reused; otherwise, one is generated. Startup and background error logs without a request context use a process correlation ID.

Tracing works locally without an external collector. Set `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317` to enable batched OTLP gRPC export. Use `http://` for a local plaintext collector and `https://` for TLS. `OTEL_SERVICE_NAME` defaults to `amartha-loan-service`. The API flushes and shuts down the TracerProvider on exit.

## Local Debugging in GoLand

```sh
docker compose up -d db
docker compose stop api
```

Open `cmd/server/main.go` in GoLand and click **Debug**. The application loads `.env` automatically, so no manual IDE environment configuration is needed. Existing process environment variables override `.env`. Send requests to `http://localhost:8080`; the debugger's listening port is separate from the API port.
