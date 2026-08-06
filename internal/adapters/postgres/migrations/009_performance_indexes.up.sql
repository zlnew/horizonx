-- 009_performance_indexes.up.sql
-- Targeted indexes for the query patterns observed in production usage:
--   * jobs   filtered by application_id / deployment_id / trace_id / status
--            (the pending-queue sort reads (server_id, status) + queued_at)
--   * logs   filtered by job_id / deployment_id / application_id / server_id
--            (detail pages pull per-job and per-deployment log tails)
--   * deployments filtered by (application_id, status) and triggered_at
-- Indexes are additive and safe to create on existing data (CREATE INDEX IF NOT EXISTS).

CREATE INDEX IF NOT EXISTS idx_jobs_application_id ON jobs (application_id);
CREATE INDEX IF NOT EXISTS idx_jobs_deployment_id ON jobs (deployment_id);
CREATE INDEX IF NOT EXISTS idx_jobs_trace_id ON jobs (trace_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status_queued_at ON jobs (status, queued_at);

CREATE INDEX IF NOT EXISTS idx_logs_job_id ON logs (job_id);
CREATE INDEX IF NOT EXISTS idx_logs_deployment_id ON logs (deployment_id);
CREATE INDEX IF NOT EXISTS idx_logs_application_id ON logs (application_id);
CREATE INDEX IF NOT EXISTS idx_logs_server_id ON logs (server_id);

CREATE INDEX IF NOT EXISTS idx_deployments_app_status ON deployments (application_id, status);
CREATE INDEX IF NOT EXISTS idx_deployments_triggered_at ON deployments (triggered_at DESC);
