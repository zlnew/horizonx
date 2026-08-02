-- P3: audit log + deployment diff support.
-- audit_logs: append-only record of user + system actions.
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT,
    actor_email VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(100),
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT fk_audit_actor FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

-- deployments: snapshot the env vars that were used for this deployment, and
-- remember the previous deployment id so the dashboard can render a diff.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS env_snapshot JSONB NOT NULL DEFAULT '{}';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS previous_deployment_id BIGINT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS previous_commit_hash VARCHAR(40);
