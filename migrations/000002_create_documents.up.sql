CREATE TYPE document_status AS ENUM (
    'uploaded',
    'queued',
    'processing',
    'completed',
    'failed'
);

CREATE TYPE document_type AS ENUM (
    'invoice',
    'contract',
    'identity',
    'financial',
    'receipt',
    'other'
);

CREATE TABLE IF NOT EXISTS documents (
    id                       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename                 VARCHAR(255) NOT NULL,  -- original filename, display only
    storage_path             VARCHAR(512) NOT NULL,  -- UUID-based path, never original filename
    file_type                VARCHAR(20) NOT NULL,   -- pdf, png, jpg, txt
    file_size                BIGINT NOT NULL,        -- bytes
    status                   document_status NOT NULL DEFAULT 'uploaded',
    document_type            document_type,          -- set after AI classification
    classification_confidence FLOAT,                 -- 0.0 to 1.0
    summary                  TEXT,                   -- AI-generated summary
    error_message            TEXT,                   -- populated on failure
    webhook_url              TEXT,                   -- HTTPS only, validated before insert
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Multi-tenant isolation: all queries filter by user_id
CREATE INDEX IF NOT EXISTS idx_documents_user_id ON documents(user_id);

-- Filter by status for job polling
CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);

-- Filter by document type for search
CREATE INDEX IF NOT EXISTS idx_documents_type ON documents(document_type);

-- Composite index for listing documents per user ordered by date
CREATE INDEX IF NOT EXISTS idx_documents_user_created ON documents(user_id, created_at DESC);