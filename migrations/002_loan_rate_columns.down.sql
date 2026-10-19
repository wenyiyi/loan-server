BEGIN;
ALTER TABLE loans DROP COLUMN updated_at;
ALTER TABLE loans RENAME COLUMN rate TO interest_rate_bps;
ALTER TABLE loans RENAME COLUMN roi TO roi_bps;
COMMIT;
