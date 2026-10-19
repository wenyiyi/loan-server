-- Fresh database schema matching internal/model. Application timestamps are Unix milliseconds.
BEGIN;

CREATE TABLE loans (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    borrower_id text NOT NULL,
    principal_amount bigint NOT NULL,
    invested_amount bigint NOT NULL DEFAULT 0,
    rate numeric(10,6) NOT NULL,
    roi numeric(10,6) NOT NULL,
    agreement_url text NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'PROPOSED',
    created_at bigint,
    updated_at bigint
);
CREATE UNIQUE INDEX idx_loans_loan_no ON loans (loan_no);

CREATE TABLE approvals (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    employee_id text NOT NULL,
    visit_proof_url text NOT NULL,
    approved_at bigint NOT NULL,
    created_at bigint,
    updated_at bigint
);
CREATE INDEX idx_approvals_loan_no ON approvals (loan_no);

CREATE TABLE disbursements (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    officer_employee_id text NOT NULL,
    signed_agreement_url text NOT NULL,
    disbursed_at bigint NOT NULL,
    created_at bigint,
    updated_at bigint
);
CREATE UNIQUE INDEX idx_disbursements_loan_no ON disbursements (loan_no);

CREATE TABLE investments (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    investor_id text NOT NULL,
    email text NOT NULL,
    amount bigint NOT NULL,
    idempotency_key text NOT NULL,
    response jsonb NOT NULL,
    created_at bigint,
    updated_at bigint
);
CREATE INDEX idx_investments_loan_no ON investments (loan_no);
CREATE UNIQUE INDEX idx_investments_idempotency_key ON investments (idempotency_key);

CREATE TABLE loan_status_history (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    previous_state text,
    new_state text,
    action text,
    created_at bigint
);
CREATE INDEX idx_loan_status_history_loan_no ON loan_status_history (loan_no);

CREATE TABLE notification_outbox (
    id bigserial PRIMARY KEY,
    loan_no varchar(22) NOT NULL,
    investor_id text NOT NULL,
    email text NOT NULL,
    agreement_url text NOT NULL,
    notification_type text NOT NULL DEFAULT 'LOAN_FUNDED',
    lease_token text,
    lease_until bigint,
    attempts bigint NOT NULL DEFAULT 0,
    available_at bigint NOT NULL,
    delivered_at bigint
);
CREATE UNIQUE INDEX uq_outbox_notification
    ON notification_outbox (loan_no, investor_id, notification_type);

COMMIT;
