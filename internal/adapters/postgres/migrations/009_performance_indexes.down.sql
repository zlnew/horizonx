-- 009_performance_indexes.down.sql
-- Drop the indexes added in 009. Safe to run: each is additive-only and
-- drop-if-exists, so this simply returns the schema to the pre-009 state.

DROP INDEX IF EXISTS idx_jobs_application_id;
DROP INDEX IF EXISTS idx_jobs_deployment_id;
DROP INDEX IF EXISTS idx_jobs_trace_id;
DROP INDEX IF EXISTS idx_jobs_status_queued_at;

DROP INDEX IF EXISTS idx_logs_job_id;
DROP INDEX IF EXISTS idx_logs_deployment_id;
DROP INDEX IF EXISTS idx_logs_application_id;
DROP INDEX IF EXISTS idx_logs_server_id;

DROP INDEX IF EXISTS idx_deployments_app_status;
DROP INDEX IF EXISTS idx_deployments_triggered_at;
