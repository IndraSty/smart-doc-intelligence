CREATE TABLE IF NOT EXISTS extractions (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id      UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    fields           JSONB NOT NULL DEFAULT '{}', -- extracted key-value fields per doc type
    raw_ai_response  TEXT NOT NULL,               -- full Gemini response, stored for debugging
    embedding        vector(768),                 -- text-embedding-004 output dimension
    search_vector    TSVECTOR,                    -- for full-text search
    processed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One extraction result per document
CREATE UNIQUE INDEX IF NOT EXISTS idx_extractions_document_id ON extractions(document_id);

-- IVFFlat index for fast approximate nearest neighbor search
-- lists=100 is a good starting point for up to ~1M vectors
CREATE INDEX IF NOT EXISTS idx_extractions_embedding
    ON extractions USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- GIN index for fast full-text search queries
CREATE INDEX IF NOT EXISTS idx_extractions_search_vector
    ON extractions USING GIN(search_vector);

-- Trigger to automatically update search_vector from fields JSONB
CREATE OR REPLACE FUNCTION extractions_search_vector_update()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector(
        'english',
        COALESCE(NEW.fields::text, '')
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER extractions_search_vector_trigger
    BEFORE INSERT OR UPDATE ON extractions
    FOR EACH ROW EXECUTE FUNCTION extractions_search_vector_update();