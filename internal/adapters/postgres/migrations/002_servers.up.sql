CREATE TABLE IF NOT EXISTS servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    ip_address INET NOT NULL,
    api_token VARCHAR(255) NOT NULL UNIQUE,
    is_online BOOLEAN DEFAULT FALSE,
    os_info JSONB,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS server_metrics (
    id BIGSERIAL PRIMARY KEY,
    server_id UUID NOT NULL,
    cpu_usage_percent DOUBLE PRECISION,
    memory_usage_percent DOUBLE PRECISION,
    data JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_metric_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_metrics_recorded_at ON server_metrics (recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_server_recorded ON server_metrics (server_id, recorded_at DESC);

COMMENT ON COLUMN server_metrics.data IS 'Full metrics JSON payload including CPU, Memory, GPU, Disk, Network';
COMMENT ON COLUMN server_metrics.cpu_usage_percent IS 'Extracted CPU usage for quick filtering';
COMMENT ON COLUMN server_metrics.memory_usage_percent IS 'Extracted memory usage for quick filtering';
