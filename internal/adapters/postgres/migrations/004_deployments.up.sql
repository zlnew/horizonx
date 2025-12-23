CREATE TABLE IF NOT EXISTS deployments (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL,
    branch VARCHAR(100) NOT NULL,
    commit_hash VARCHAR(40),
    commit_message TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'building', 'deploying', 'running', 'failed')),
    build_logs TEXT,

    deployed_by BIGINT,

    triggered_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    CONSTRAINT fk_deployment_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
    CONSTRAINT fk_deployment_author FOREIGN KEY (deployed_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_deployments_app_id ON deployments(application_id);
