CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    api_key       VARCHAR(64) NOT NULL UNIQUE, -- stores SHA-256 hash of the actual key
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast email lookup during login
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Index for fast API key lookup during authentication
CREATE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key);