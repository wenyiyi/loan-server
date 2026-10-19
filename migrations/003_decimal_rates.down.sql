BEGIN;
ALTER TABLE loans DROP CONSTRAINT loans_rate_check;
ALTER TABLE loans DROP CONSTRAINT loans_roi_check;
ALTER TABLE loans ALTER COLUMN rate TYPE INTEGER USING round(rate * 100)::integer;
ALTER TABLE loans ALTER COLUMN roi TYPE INTEGER USING round(roi * 100)::integer;
ALTER TABLE loans ADD CONSTRAINT loans_interest_rate_bps_check CHECK (rate BETWEEN 0 AND 10000);
ALTER TABLE loans ADD CONSTRAINT loans_roi_bps_check CHECK (roi >= 0 AND roi <= rate);
COMMIT;
