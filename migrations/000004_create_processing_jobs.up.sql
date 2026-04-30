CREATE TYPE job_status AS ENUM (
    'queued',
    'processing',
    'completed',
    'failed'
);

CREATE TABLE IF NOT EXISTS processing_jobs (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    status       job_status NOT NULL DEFAULT 'queued',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    queued_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- One job per document
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_document_id ON processing_jobs(document_id);

-- Filter jobs by status for monitoring
CREATE INDEX IF NOT EXISTS idx_jobs_status ON processing_jobs(status);