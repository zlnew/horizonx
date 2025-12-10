CREATE TABLE IF NOT EXISTS servers (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    ip_address INET NOT NULL,
    api_token VARCHAR(255),
    is_online BOOLEAN DEFAULT false,
    os_info JSONB,
    created_at TIMESTAMP(0) DEFAULT NOW(),
    updated_at TIMESTAMP(0) DEFAULT NOW(),
    deleted_at TIMESTAMP(0) DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS server_metrics (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    cpu_usage_percent DOUBLE PRECISION,
    memory_usage_percent DOUBLE PRECISION,
    data JSONB NOT NULL,
    recorded_at TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_metric_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

CREATE INDEX idx_metrics_server_time ON server_metrics (server_id, recorded_at DESC);
