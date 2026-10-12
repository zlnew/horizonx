-- 013_jobs_logs_retention: time-based retention for jobs and logs.
-- jobs is purged on queued_at (new standalone index: the existing
-- idx_jobs_server_status / idx_jobs_status_queued_at cannot serve
-- WHERE queued_at < $1). logs reuses idx_logs_timestamp (timestamp DESC).

CREATE INDEX IF NOT EXISTS idx_jobs_queued_at ON jobs (queued_at);
