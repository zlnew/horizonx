CREATE TABLE IF NOT EXISTS server_jobs (
    id BIGSERIAL PRIMARY KEY,
    server_id UUID NOT NULL,
    application_id BIGINT,
    job_type VARCHAR(50) NOT NULL,
    command_payload JSONB,
    status VARCHAR(20) DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    output_log TEXT,

    queued_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    CONSTRAINT fk_job_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE,
    CONSTRAINT fk_job_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE SET NULL
);

CREATE INDEX idx_jobs_server_status ON server_jobs (server_id, status);
