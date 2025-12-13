CREATE TABLE IF NOT EXISTS applications (
    id BIGSERIAL PRIMARY KEY,
    server_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    repo_url VARCHAR(255),
    branch VARCHAR(100) DEFAULT 'main',

    docker_compose_raw TEXT, 

    status VARCHAR(20) DEFAULT 'stopped' CHECK (status IN ('stopped', 'starting', 'running', 'restarting', 'failed')),
    last_deployment_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,

    CONSTRAINT fk_app_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS environment_variables (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    is_preview BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT fk_env_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
    UNIQUE (application_id, key)
);

CREATE TABLE IF NOT EXISTS volumes (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL,
    host_path VARCHAR(255) NOT NULL,
    container_path VARCHAR(255) NOT NULL,
    mode VARCHAR(10) DEFAULT 'rw',
    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT fk_volume_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
);

CREATE INDEX idx_apps_server_id ON applications(server_id);
CREATE INDEX idx_env_app_id ON environment_variables(application_id);
