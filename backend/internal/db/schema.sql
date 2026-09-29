CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS layers (
    id BIGSERIAL PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    hash_namespace TEXT NOT NULL,
    salt TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS experiments (
    id BIGSERIAL PRIMARY KEY,
    layer_id BIGINT NOT NULL REFERENCES layers(id) ON DELETE CASCADE,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    bucket_start INTEGER NOT NULL CHECK (bucket_start >= 0 AND bucket_start < 10000),
    bucket_end INTEGER NOT NULL CHECK (bucket_end >= bucket_start AND bucket_end < 10000),
    targeting JSONB NOT NULL DEFAULT '{}'::jsonb,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT experiments_layer_range_excl EXCLUDE USING gist (
        layer_id WITH =,
        int4range(bucket_start, bucket_end + 1, '[)') WITH &&
    ) WHERE (active)
);

CREATE TABLE IF NOT EXISTS variants (
    id BIGSERIAL PRIMARY KEY,
    experiment_id BIGINT NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    weight INTEGER NOT NULL CHECK (weight >= 0 AND weight <= 10000),
    UNIQUE (experiment_id, key)
);

CREATE TABLE IF NOT EXISTS whitelist (
    id BIGSERIAL PRIMARY KEY,
    layer_id BIGINT NOT NULL REFERENCES layers(id) ON DELETE CASCADE,
    experiment_id BIGINT NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    variant_key TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (experiment_id, user_id),
    UNIQUE (layer_id, user_id)
);

CREATE TABLE IF NOT EXISTS assignments (
    id BIGSERIAL PRIMARY KEY,
    layer_id BIGINT NOT NULL REFERENCES layers(id) ON DELETE CASCADE,
    experiment_id BIGINT NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    bucket INTEGER NOT NULL CHECK (bucket >= 0 AND bucket < 10000),
    variant_key TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('bucket', 'whitelist')),
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (layer_id, user_id)
);

CREATE INDEX IF NOT EXISTS assignments_experiment_idx
    ON assignments (experiment_id, variant_key, source);

CREATE TABLE IF NOT EXISTS exposures (
    id BIGSERIAL PRIMARY KEY,
    layer_id BIGINT NOT NULL REFERENCES layers(id) ON DELETE CASCADE,
    experiment_id BIGINT NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    assignment_id BIGINT REFERENCES assignments(id) ON DELETE SET NULL,
    user_id TEXT NOT NULL,
    bucket INTEGER,
    variant_key TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('bucket', 'whitelist')),
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS exposures_stats_idx
    ON exposures (experiment_id, variant_key, source);
