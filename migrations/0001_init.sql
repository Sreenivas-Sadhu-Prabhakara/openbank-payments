-- Payments service schema. Owned exclusively by the payments microservice;
-- no other service reads or writes these tables.
CREATE SCHEMA IF NOT EXISTS payments;

CREATE TABLE IF NOT EXISTS payments.domestic_payments (
    id                  TEXT PRIMARY KEY,
    consent_id          TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    creation_dt         TIMESTAMPTZ NOT NULL,
    status_update_dt    TIMESTAMPTZ NOT NULL,

    -- Caller's x-idempotency-key. Unique so a retried POST maps to one payment.
    idempotency_key     TEXT UNIQUE,

    -- Initiation (mirrors the domestic-payment-consent Initiation).
    instruction_id      TEXT,
    e2e_id              TEXT,
    instructed_amount   TEXT        NOT NULL,
    instructed_currency TEXT        NOT NULL,
    creditor_scheme     TEXT        NOT NULL,
    creditor_ident      TEXT        NOT NULL,
    creditor_name       TEXT,
    reference           TEXT,

    -- Optional debtor account.
    debtor_scheme       TEXT,
    debtor_ident        TEXT,
    debtor_name         TEXT
);

CREATE INDEX IF NOT EXISTS idx_domestic_payments_consent ON payments.domestic_payments (consent_id);
