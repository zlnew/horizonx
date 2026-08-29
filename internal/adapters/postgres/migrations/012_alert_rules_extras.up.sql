-- 012_alert_rules_extras: operational knobs on alert_rules, added after 011.
-- cooldown       = minimum seconds between two firings of the same rule
--                  (prevents notification spam on flapping conditions)
-- silenced_until = optional timestamp during which the rule must not fire
--                  (set via POST /alerts/{id}/silence on the rule behind an
--                  active alert)

ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS cooldown INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS silenced_until TIMESTAMPTZ NULL;