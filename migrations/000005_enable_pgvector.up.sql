-- Enable pgvector extension for semantic search
-- Must be run before creating vector columns
-- Supabase already has this extension available
CREATE EXTENSION IF NOT EXISTS vector;