BEGIN;
ALTER TABLE loans DROP CONSTRAINT loans_interest_rate_bps_check;
ALTER TABLE loans DROP CONSTRAINT loans_check;
ALTER TABLE loans ALTER COLUMN rate TYPE NUMERIC(10,6) USING rate::numeric / 100;
ALTER TABLE loans ALTER COLUMN roi TYPE NUMERIC(10,6) USING roi::numeric / 100;
ALTER TABLE loans ADD CONSTRAINT loans_rate_check CHECK (rate BETWEEN 0 AND 100);
ALTER TABLE loans ADD CONSTRAINT loans_roi_check CHECK (roi >= 0 AND roi <= rate);
COMMIT;
