# openbank-payments

[![CI](https://github.com/Sreenivas-Sadhu-Prabhakara/openbank-payments/actions/workflows/ci.yml/badge.svg)](https://github.com/Sreenivas-Sadhu-Prabhakara/openbank-payments/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)

The **Payments (PIS)** microservice — the BIAN *Payment Order / Payment Execution* service domain, exposing the OBIE **PISP** domestic-payment APIs.

A payment can only be created from an **authorised** `domestic-payment` consent. The service validates the consent against the consent service, checks the request `Initiation` matches the consent exactly, enforces the mandatory `x-idempotency-key`, and then marks the single-use consent `Consumed`.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/domestic-payments` | Create a payment (requires `x-idempotency-key`) |
| GET | `/domestic-payments/{domesticPaymentId}` | Read a payment |
| GET | `/domestic-payments/{domesticPaymentId}/payment-details` | Payment status detail |
| GET | `/health` | Liveness |

Key rules: missing `x-idempotency-key` → 400; replay of a known key returns the original payment (no duplicate, consent consumed once); consent not `Authorised`/wrong type → 403; `InstructedAmount` mismatch vs. consent → 400 (`UK.OBIE.Resource.ConsentMismatch`).

## Configuration

| Env | Default | Notes |
|---|---|---|
| `ADDR` | `:8083` | Listen address |
| `BASE_URL` | `http://localhost:8083` | Used for `Links.Self` |
| `DATABASE_URL` | _(unset)_ | Postgres DSN; **unset → in-memory store** |
| `CONSENT_URL` | `http://localhost:8081` | Consent service base URL |

## Run

```bash
go run .                              # in-memory
docker build -t openbank/payments . && docker run -p 8083:8083 openbank/payments
```

## Test

```bash
go test ./...                       # unit + handler tests (fake consent client, no Docker)
go test -tags=integration ./...     # Postgres repo tests via testcontainers (needs Docker)
```

## Layout notes

- `internal/payments/` — domain, `Repository` port (in-memory + Postgres, with idempotency-key lookup), service logic, OBIE handlers.
- `migrations/` — SQL owned by this service, applied on startup when `DATABASE_URL` is set.
- `pkg/` — vendored shared OBIE library, wired via `replace ... => ./pkg`.
- Ordering guarantee: the consent is consumed **before** the payment is persisted, so a failed consume never leaves an orphan accepted payment.
