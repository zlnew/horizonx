-- 011_alert_rules: rule definitions + persisted alert history
--
-- alert_rules holds the rules a user configures (metric thresholds, health
-- target statuses, offline detection). alert_history records every fire and
-- resolution with the observed value at firing time. An alert is "active"
-- when the latest row for its (rule_id, server_id, app_id) group is in state
-- 'firing'.

CREATE TABLE IF NOT EXISTS alert_rules (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    scope         TEXT NOT NULL CHECK (scope IN ('server', 'app', 'global')),
    server_id     UUID NULL,
    app_id        BIGINT NULL,
    source        TEXT NOT NULL CHECK (source IN ('metric', 'health', 'offline')),
    metric_path   TEXT NULL,
    operator      TEXT NULL CHECK (operator IN ('>', '>=', '<', '<=')),
    threshold     DOUBLE PRECISION NULL,
    target_status TEXT NULL,
    for_duration  INT NOT NULL DEFAULT 0,
    severity      TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('info', 'warning', 'critical')),
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_alert_rule_scope_bindings CHECK (
        (scope = 'server' AND server_id IS NOT NULL AND app_id IS NULL)
        OR (scope = 'app' AND app_id IS NOT NULL AND server_id IS NULL)
        OR (scope = 'global' AND server_id IS NULL AND app_id IS NULL)
    ),
    -- A metric rule must carry a resolvable path + comparison operator +
    -- threshold; a health rule must carry the target status it reacts to.
    CONSTRAINT chk_alert_rule_source_params CHECK (
        (source = 'metric'  AND metric_path IS NOT NULL AND metric_path <> '' AND operator IS NOT NULL AND threshold IS NOT NULL)
        OR (source = 'health'  AND target_status IS NOT NULL AND target_status <> '')
        OR (source = 'offline')
    )
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules (enabled);
CREATE INDEX IF NOT EXISTS idx_alert_rules_scope ON alert_rules (scope, server_id, app_id);

CREATE TABLE IF NOT EXISTS alert_history (
    id             BIGSERIAL PRIMARY KEY,
    rule_id        BIGINT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    server_id      UUID NOT NULL,
    app_id         BIGINT NULL,
    severity       TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    state          TEXT NOT NULL DEFAULT 'firing' CHECK (state IN ('firing', 'resolved')),
    value          DOUBLE PRECISION NULL,
    message        TEXT NOT NULL DEFAULT '',
    acked          BOOLEAN NOT NULL DEFAULT FALSE,
    silenced_until TIMESTAMPTZ NULL,
    first_fired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at    TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_state ON alert_history (state);
CREATE INDEX IF NOT EXISTS idx_alert_history_rule_group ON alert_history (rule_id, server_id, app_id);
CREATE INDEX IF NOT EXISTS idx_alert_history_created_at ON alert_history (created_at DESC);