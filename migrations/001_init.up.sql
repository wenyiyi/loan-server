BEGIN;
CREATE TABLE loans (
 id BIGSERIAL PRIMARY KEY, borrower_id TEXT NOT NULL,
 principal_amount BIGINT NOT NULL CHECK(principal_amount>0),
 invested_amount BIGINT NOT NULL DEFAULT 0 CHECK(invested_amount>=0 AND invested_amount<=principal_amount),
 interest_rate_bps INTEGER NOT NULL CHECK(interest_rate_bps BETWEEN 0 AND 10000),
 roi_bps INTEGER NOT NULL CHECK(roi_bps>=0 AND roi_bps<=interest_rate_bps),
 agreement_url TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'PROPOSED' CHECK(status IN ('PROPOSED','APPROVED','INVESTED','DISBURSED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK((status IN ('INVESTED','DISBURSED') AND invested_amount=principal_amount) OR (status IN ('PROPOSED','APPROVED') AND invested_amount<principal_amount)),
 CHECK(status<>'PROPOSED' OR invested_amount=0)
);
CREATE TABLE loan_approvals(loan_id BIGINT PRIMARY KEY REFERENCES loans(id), validator_employee_id TEXT NOT NULL, visit_proof_url TEXT NOT NULL, approved_at TIMESTAMPTZ NOT NULL);
CREATE TABLE loan_disbursements(loan_id BIGINT PRIMARY KEY REFERENCES loans(id), officer_employee_id TEXT NOT NULL, signed_agreement_url TEXT NOT NULL, disbursed_at TIMESTAMPTZ NOT NULL);
CREATE TABLE loan_investments(id BIGSERIAL PRIMARY KEY, loan_id BIGINT NOT NULL REFERENCES loans(id), investor_id TEXT NOT NULL, email TEXT NOT NULL, amount BIGINT NOT NULL CHECK(amount>0), idempotency_key TEXT NOT NULL UNIQUE, response JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX idx_investments_loan_id ON loan_investments(loan_id);
CREATE TABLE loan_status_history(id BIGSERIAL PRIMARY KEY, loan_id BIGINT NOT NULL REFERENCES loans(id), previous_state TEXT, new_state TEXT NOT NULL, action TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX idx_history_loan_id ON loan_status_history(loan_id);
CREATE TABLE notification_outbox(id BIGSERIAL PRIMARY KEY, loan_id BIGINT NOT NULL REFERENCES loans(id), investor_id TEXT NOT NULL, email TEXT NOT NULL, agreement_url TEXT NOT NULL, notification_type TEXT NOT NULL DEFAULT 'LOAN_FUNDED', lease_token TEXT, attempts INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(), delivered_at TIMESTAMPTZ, UNIQUE(loan_id,investor_id,notification_type));
CREATE INDEX idx_outbox_pending ON notification_outbox(available_at) WHERE delivered_at IS NULL;
COMMIT;
