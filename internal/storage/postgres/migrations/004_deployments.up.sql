CREATE TABLE IF NOT EXISTS deployments (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL,
    commit_hash VARCHAR(40),
    commit_message TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'building', 'deploying', 'running', 'failed')),
    build_logs TEXT,

    started_at TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ,

    CONSTRAINT fk_deployment_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployments_app_id ON deployments(application_id);
