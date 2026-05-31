-- Subsite Relay: observability, sticky routing, health samples, and circuit breakers.
CREATE TABLE IF NOT EXISTS subsite_forward_affinities (
    id                   BIGSERIAL PRIMARY KEY,
    affinity_key         VARCHAR(256) NOT NULL UNIQUE,
    affinity_type        VARCHAR(32) NOT NULL DEFAULT 'session',
    subsite_id           VARCHAR(64) NOT NULL REFERENCES subsites(subsite_id) ON DELETE CASCADE,
    lease_id             VARCHAR(80),
    account_id           BIGINT,
    api_key_id           BIGINT,
    user_id              BIGINT,
    group_id             BIGINT,
    model                VARCHAR(160) NOT NULL DEFAULT '',
    session_id           VARCHAR(256) NOT NULL DEFAULT '',
    source               VARCHAR(32) NOT NULL DEFAULT 'auto',
    locked               BOOLEAN NOT NULL DEFAULT FALSE,
    hits                 BIGINT NOT NULL DEFAULT 0,
    last_reason          VARCHAR(64) NOT NULL DEFAULT '',
    last_error           TEXT NOT NULL DEFAULT '',
    expires_at           TIMESTAMPTZ NOT NULL,
    last_used_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ,
    CONSTRAINT subsite_forward_affinities_type_check CHECK (affinity_type IN ('session', 'api_key', 'account', 'manual')),
    CONSTRAINT subsite_forward_affinities_source_check CHECK (source IN ('auto', 'manual', 'fallback', 'imported'))
);

CREATE INDEX IF NOT EXISTS idx_subsite_forward_affinities_subsite
    ON subsite_forward_affinities (subsite_id, last_used_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_subsite_forward_affinities_api_key
    ON subsite_forward_affinities (api_key_id, last_used_at DESC)
    WHERE deleted_at IS NULL AND api_key_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_subsite_forward_affinities_account
    ON subsite_forward_affinities (account_id, last_used_at DESC)
    WHERE deleted_at IS NULL AND account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_subsite_forward_affinities_expires
    ON subsite_forward_affinities (expires_at)
    WHERE deleted_at IS NULL AND locked = FALSE;

CREATE TABLE IF NOT EXISTS subsite_forward_events (
    id                   BIGSERIAL PRIMARY KEY,
    request_id           VARCHAR(128) NOT NULL DEFAULT '',
    affinity_key         VARCHAR(256) NOT NULL DEFAULT '',
    subsite_id           VARCHAR(64) REFERENCES subsites(subsite_id) ON DELETE SET NULL,
    attempted_subsite_id VARCHAR(64) NOT NULL DEFAULT '',
    fallback_from        VARCHAR(64) NOT NULL DEFAULT '',
    lease_id             VARCHAR(80),
    account_id           BIGINT,
    api_key_id           BIGINT,
    user_id              BIGINT,
    group_id             BIGINT,
    model                VARCHAR(160) NOT NULL DEFAULT '',
    session_id           VARCHAR(256) NOT NULL DEFAULT '',
    method               VARCHAR(16) NOT NULL DEFAULT '',
    path                 TEXT NOT NULL DEFAULT '',
    status_code          INT NOT NULL DEFAULT 0,
    latency_ms           BIGINT NOT NULL DEFAULT 0,
    request_bytes        BIGINT NOT NULL DEFAULT 0,
    response_bytes       BIGINT NOT NULL DEFAULT 0,
    reason               VARCHAR(64) NOT NULL DEFAULT '',
    outcome              VARCHAR(32) NOT NULL DEFAULT '',
    error                TEXT NOT NULL DEFAULT '',
    metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subsite_forward_events_outcome_check CHECK (outcome IN ('success', 'failed', 'no_candidate', 'fallback', 'client_error', 'upstream_error'))
);

CREATE INDEX IF NOT EXISTS idx_subsite_forward_events_created
    ON subsite_forward_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_subsite_forward_events_subsite_created
    ON subsite_forward_events (subsite_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_subsite_forward_events_affinity_created
    ON subsite_forward_events (affinity_key, created_at DESC);

CREATE TABLE IF NOT EXISTS subsite_health_samples (
    id                   BIGSERIAL PRIMARY KEY,
    subsite_id           VARCHAR(64) NOT NULL REFERENCES subsites(subsite_id) ON DELETE CASCADE,
    status               VARCHAR(32) NOT NULL DEFAULT '',
    health_score         INT NOT NULL DEFAULT 0,
    active_requests      INT NOT NULL DEFAULT 0,
    queued_usage         INT NOT NULL DEFAULT 0,
    qps                  DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_percent          DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_bytes         BIGINT NOT NULL DEFAULT 0,
    latency_ms           BIGINT NOT NULL DEFAULT 0,
    error_rate           DOUBLE PRECISION NOT NULL DEFAULT 0,
    version              VARCHAR(64) NOT NULL DEFAULT '',
    metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subsite_health_samples_subsite_created
    ON subsite_health_samples (subsite_id, created_at DESC);

CREATE TABLE IF NOT EXISTS subsite_circuit_breakers (
    id                   BIGSERIAL PRIMARY KEY,
    scope                VARCHAR(32) NOT NULL,
    target_id            VARCHAR(128) NOT NULL,
    subsite_id           VARCHAR(64) REFERENCES subsites(subsite_id) ON DELETE CASCADE,
    account_id           BIGINT,
    lease_id             VARCHAR(80),
    reason               TEXT NOT NULL DEFAULT '',
    failures             INT NOT NULL DEFAULT 0,
    cooldown_until       TIMESTAMPTZ NOT NULL,
    last_error           TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ,
    CONSTRAINT subsite_circuit_breakers_scope_check CHECK (scope IN ('subsite', 'account', 'lease')),
    CONSTRAINT subsite_circuit_breakers_target_unique UNIQUE (scope, target_id)
);

CREATE INDEX IF NOT EXISTS idx_subsite_circuit_breakers_active
    ON subsite_circuit_breakers (cooldown_until DESC)
    WHERE deleted_at IS NULL;

INSERT INTO settings (key, value, updated_at)
VALUES ('subsite_forward_mode', 'forward', NOW())
ON CONFLICT (key) DO NOTHING;
