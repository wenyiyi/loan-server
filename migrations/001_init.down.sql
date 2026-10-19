-- Destructive rollback. Execute explicitly; never mount in docker-entrypoint-initdb.d.
BEGIN;
DROP TABLE notification_outbox;
DROP TABLE loan_status_history;
DROP TABLE investments;
DROP TABLE disbursements;
DROP TABLE approvals;
DROP TABLE loans;
COMMIT;
