-- P3: audit log + deployment diff support (down).
DROP TABLE IF EXISTS audit_logs;
ALTER TABLE deployments DROP COLUMN IF EXISTS env_snapshot;
ALTER TABLE deployments DROP COLUMN IF EXISTS previous_deployment_id;
ALTER TABLE deployments DROP COLUMN IF EXISTS previous_commit_hash;
