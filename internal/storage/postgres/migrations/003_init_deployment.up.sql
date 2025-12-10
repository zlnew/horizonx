CREATE TABLE IF NOT EXISTS applications (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    repo_url VARCHAR(255),
    docker_compose_raw TEXT,
    env_vars TEXT,
    status VARCHAR(20) DEFAULT 'stopped',
    created_at TIMESTAMP(0) DEFAULT NOW(),
    updated_at TIMESTAMP(0) DEFAULT NOW(),
    deleted_at TIMESTAMP(0) DEFAULT NULL,
    CONSTRAINT fk_app_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);
