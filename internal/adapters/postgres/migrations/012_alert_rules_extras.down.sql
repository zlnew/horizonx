ALTER TABLE alert_rules
    DROP COLUMN IF EXISTS silenced_until,
    DROP COLUMN IF EXISTS cooldown;