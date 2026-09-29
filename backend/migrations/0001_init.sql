-- Stable experiment definitions and immutable allocation/exposure records.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS layers (
    id TEXT PRIMARY KEY,
    namespace TEXT NOT NULL,
    name TEXT NOT NULL,
    salt TEXT NOT NULL,
    bucket_size INTEGER NOT NULL CHECK (bucket_size > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS experiments (
    id TEXT PRIMARY KEY,
    layer_id TEXT NOT NULL REFERENCES layers(id),
    name TEXT NOT NULL,
    start_bucket INTEGER NOT NULL,
    end_bucket INTEGER NOT NULL,
    salt TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    criteria JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (start_bucket >= 0),
    CHECK (end_bucket > start_bucket),
    EXCLUDE USING gist (
        layer_id WITH =,
        int4range(start_bucket, end_bucket, '[)') WITH &&
    ) WHERE (active)
);

CREATE TABLE IF NOT EXISTS variants (
    id TEXT PRIMARY KEY,
    experiment_id TEXT NOT NULL REFERENCES experiments(id),
    name TEXT NOT NULL,
    start_bucket INTEGER NOT NULL,
    end_bucket INTEGER NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (start_bucket >= 0),
    CHECK (end_bucket > start_bucket),
    EXCLUDE USING gist (
        experiment_id WITH =,
        int4range(start_bucket, end_bucket, '[)') WITH &&
    ) WHERE (active)
);

CREATE TABLE IF NOT EXISTS whitelists (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    layer_id TEXT NOT NULL REFERENCES layers(id),
    experiment_id TEXT NOT NULL REFERENCES experiments(id),
    variant_id TEXT NOT NULL REFERENCES variants(id),
    reason TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (user_id WITH =, layer_id WITH =) WHERE (active)
);

CREATE TABLE IF NOT EXISTS assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    layer_id TEXT NOT NULL REFERENCES layers(id),
    experiment_id TEXT REFERENCES experiments(id),
    variant_id TEXT REFERENCES variants(id),
    bucket INTEGER NOT NULL,
    variant_bucket INTEGER,
    status TEXT NOT NULL CHECK (status IN ('assigned', 'rejected')),
    reject_reason TEXT,
    override_reason TEXT,
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, layer_id, source),
    CHECK (
        (status = 'assigned' AND experiment_id IS NOT NULL AND variant_id IS NOT NULL)
        OR status = 'rejected'
    )
);

CREATE INDEX IF NOT EXISTS idx_assignments_source_status ON assignments(source, status);

CREATE TABLE IF NOT EXISTS exposures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    experiment_id TEXT NOT NULL REFERENCES experiments(id),
    variant_id TEXT NOT NULL REFERENCES variants(id),
    source TEXT NOT NULL,
    override_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (assignment_id)
);

CREATE INDEX IF NOT EXISTS idx_exposures_group ON exposures(source, experiment_id, variant_id);
