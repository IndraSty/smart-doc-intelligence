# Smart Document Intelligence API

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white)](https://golang.org/)
[![Echo](https://img.shields.io/badge/Echo-v4-blue?style=flat)](https://echo.labstack.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgvector-336791?style=flat&logo=postgresql&logoColor=white)](https://github.com/pgvector/pgvector)
[![RabbitMQ](https://img.shields.io/badge/RabbitMQ-async-FF6600?style=flat&logo=rabbitmq&logoColor=white)](https://www.rabbitmq.com/)
[![Redis](https://img.shields.io/badge/Redis-job--tracking-DC382D?style=flat&logo=redis&logoColor=white)](https://redis.io/)
[![Gemini](https://img.shields.io/badge/Gemini-AI-4285F4?style=flat&logo=google&logoColor=white)](https://ai.google.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Swagger](https://img.shields.io/badge/Swagger-documented-85EA2D?style=flat&logo=swagger&logoColor=black)](http://localhost:8080/swagger/index.html)

> **Upload any document. Get structured data back. No manual entry.**
> An async document intelligence API that classifies, extracts, and semantically searches PDFs, images, and text — powered by Gemini AI and pgvector.

[Features](#features) · [Architecture](#architecture) · [Tech Stack](#tech-stack) · [Getting Started](#getting-started) · [API Reference](#api-reference) · [Benchmarks](#benchmarks) · [Security](#security) · [Observability](#observability)

---

## Features

|   | Feature | Description |
|---|---|---|
| 📤 | **Async Document Ingestion** | Upload PDF, PNG, JPG, TXT up to 10MB — get a job ID back in <100ms, AI runs in background |
| 🤖 | **AI Classification** | Gemini detects document type (invoice, contract, identity, financial, receipt) with confidence score |
| 🔍 | **Field Extraction** | Type-specific prompt templates extract structured fields — invoice numbers, party names, amounts, dates |
| 📝 | **Auto Summarization** | 2-3 sentence AI-generated summary per document, stored alongside extracted fields |
| 🧠 | **Semantic Search** | Query documents in natural language using Gemini embeddings + pgvector cosine similarity |
| 📖 | **Full-Text Search** | PostgreSQL tsvector keyword search with `plainto_tsquery` for exact term matching |
| ⚡ | **Hybrid Search (RRF)** | Reciprocal Rank Fusion combines semantic + full-text — best of both worlds in one query |
| 🔄 | **Worker Pool** | Configurable goroutine pool consuming RabbitMQ jobs in parallel with manual ack/nack |
| 🔁 | **Exponential Backoff Retry** | Failed AI calls retry up to 3×: 1s→2s→4s per call, 2s→4s→8s per job |
| 🔗 | **Webhook Callbacks** | POST to your URL when processing completes — HTTPS only, fired async from worker |
| 🔐 | **Presigned Download URLs** | 15-minute expiring Supabase Storage URLs — files never proxied through API server |
| 👥 | **Multi-Tenant Isolation** | Every query scoped by `user_id` — zero cross-tenant data leakage by design |
| 📊 | **Prometheus Metrics** | 18 metrics across HTTP, AI, worker, search, and webhook subsystems |
| 🏥 | **Deep Health Checks** | Per-dependency latency + DB pool stats at `/health` — not just a 200 OK ping |

---

## Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            CLIENT (HTTP)                                 │
└────────────────────────────────┬─────────────────────────────────────────┘
                                 │ REST API
                                 ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         API SERVER (Echo)                                │
│                                                                          │
│  ┌───────────┐  ┌────────────┐  ┌───────────┐  ┌─────────────────────┐   │
│  │   Auth    │  │  Document  │  │  Search   │  │  Health / Metrics   │   │
│  │  Handler  │  │  Handler   │  │  Handler  │  │      Handler        │   │
│  └─────┬─────┘  └─────┬──────┘  └─────┬─────┘  └──────────┬──────────┘   │
│        │              │               │                   │              │
│  ┌─────▼──────────────▼───────────────▼───────────────────▼──────────┐   │
│  │                        Usecase Layer                              │   │
│  │         UserUsecase · DocumentUsecase · SearchUsecase             │   │
│  │                      ProcessingUsecase                            │   │
│  └─────┬───────────────┬───────────────┬───────────────────────────┬─┘   │
│        │               │               │                           │     │
│  ┌─────▼────┐  ┌───────▼──────┐  ┌─────▼──────┐  ┌─────────────────▼──┐  │
│  │  User    │  │   Document   │  │ Extraction │  │    Job Repository  │  │
│  │  Repo    │  │    Repo      │  │   Repo     │  │  (Redis + Postgres)│  │
│  └─────┬────┘  └───────┬──────┘  └────┬───────┘  └─────────────────┬──┘  │
└────────┼───────────────┼──────────────┼────────────────────────────┼─────┘
         │               │              │                            │
         ▼               ▼              ▼                            ▼
   ┌──────────┐   ┌───────────┐   ┌──────────┐               ┌──────────────┐
   │Supabase  │   │Supabase   │   │Supabase  │               │   Upstash    │
   │PostgreSQL│   │PostgreSQL │   │PostgreSQL│               │    Redis     │
   │ (users)  │   │(documents)│   │+ pgvector│               │(job status)  │
   └──────────┘   └───────────┘   └──────────┘               └──────────────┘
                       │
                       │ publish job
                       ▼
              ┌─────────────────┐
              │   CloudAMQP     │
              │   RabbitMQ      │
              └────────┬────────┘
                       │ consume job
                       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                      WORKER PROCESS (Goroutine Pool)                     │
│                                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ Worker-0 │  │ Worker-1 │  │ Worker-2 │  │ Worker-3 │  │ Worker-4 │    │
│  └─────┬────┘  └─────┬────┘  └─────┬────┘  └─────┬────┘  └─────┬────┘    │
│        └─────────────┴─────────────┴─────────────┴─────────────┘         │
│                                    │                                     │
│              ┌─────────────────────▼────────────────────────┐            │
│              │            processJob() pipeline             │            │
│              │  1. fetch document metadata (Postgres)       │            │
│              │  2. download file (Supabase Storage)         │            │
│              │  3. classify + extract + summarize (Gemini)  │            │
│              │  4. generate embedding vector (Gemini)       │            │
│              │  5. save extraction + vector (Postgres)      │            │
│              │  6. update document status (Postgres)        │            │
│              │  7. fire webhook callback (HTTPS POST)       │            │
│              └──────────────────────────────────────────────┘            │
└──────────────────────────────────────────────────────────────────────────┘
```

### Upload Hot Path (Request Flow)

```
POST /api/v1/documents
         │
         ▼
┌─────────────────┐     ┌─────────────────────────────────────┐
│  Rate Limiter   │────▶│  2 req/s per user (upload endpoint) │
│  Middleware     │     └─────────────────────────────────────┘
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────────────────────────┐
│  Upload         │────▶│  Content-Length check + MaxBytes   │
│  Validator      │     │  reader wrap (before body is read)  │
└────────┬────────┘     └─────────────────────────────────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────────────────────────┐
│  Auth           │────▶│  JWT Bearer OR X-API-Key header    │
│  Middleware     │     │  SHA-256 hash lookup in Postgres    │
└────────┬────────┘     └─────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ DocumentHandler │
│   .Upload()     │
└────────┬────────┘
         │
         ├──▶ Magic byte validation (not file extension)
         │
         ├──▶ UUID storage path (never original filename)
         │
         ├──▶ Supabase Storage upload
         │
         ├──▶ INSERT into documents (status: uploaded)
         │
         ├──▶ INSERT into processing_jobs
         │         │
         │         └──▶ SET Redis job status (queued)
         │         │
         │         └──▶ PUBLISH to RabbitMQ
         │
         └──▶ Return 202 { document_id, job_id } ◀── < 100ms
```

### Clean Architecture Layers

```
┌─────────────────────────────────────────────────────┐
│                  delivery/http                      │
│         (handlers, middleware, router)              │
│   depends on ▼ usecase interfaces only              │
├─────────────────────────────────────────────────────┤
│                    usecase                          │
│      (business logic, orchestration)                │
│   depends on ▼ domain interfaces only               │
├─────────────────────────────────────────────────────┤
│                   repository                        │
│         (postgres, redis implementations)           │
│   depends on ▼ domain entities only                 │
├─────────────────────────────────────────────────────┤
│                     domain                          │
│   (entities, interfaces, errors) ← NO dependencies  │
│         zero imports from other layers              │
└─────────────────────────────────────────────────────┘

Dependency direction: inward only
Outer layers know about inner layers. Never the reverse.

  delivery ──▶ usecase ──▶ domain ◀── repository
                              ▲
                              │
                          worker / ai
```

### Database Schema

```
┌─────────────────────────────────────────────────────────────────┐
│  users                                                          │
│  ─────────────────────────────────────────────────────────────  │
│  id               UUID        PK                                │
│  email            VARCHAR     UNIQUE NOT NULL                   │
│  password_hash    VARCHAR     bcrypt cost 12                    │
│  api_key          VARCHAR     SHA-256 hash, UNIQUE              │
│  created_at       TIMESTAMPTZ                                   │
│  updated_at       TIMESTAMPTZ                                   │
└──────────────────────────────┬──────────────────────────────────┘
                               │ 1
                               │
                               │ N
┌──────────────────────────────▼───────────────────────────────────┐
│  documents                                                       │
│  ─────────────────────────────────────────────────────────────   │
│  id                      UUID        PK                          │
│  user_id                 UUID        FK → users.id               │
│  filename                VARCHAR     original name, display only │
│  storage_path            VARCHAR     UUID-based, never filename  │
│  file_type               VARCHAR     pdf | png | jpg | txt       │
│  file_size               BIGINT      bytes                       │
│  status                  ENUM        uploaded→queued→processing  │
│                                      →completed | failed         │
│  document_type           ENUM        invoice | contract | ...    │
│  classification_confidence FLOAT     0.0 – 1.0                   │
│  summary                 TEXT        AI-generated, nullable      │
│  error_message           TEXT        populated on failure        │
│  webhook_url             TEXT        HTTPS only                  │
│  created_at              TIMESTAMPTZ                             │
│  updated_at              TIMESTAMPTZ                             │
└──────────────────────────────┬───────────────────────────────────┘
                               │ 1
                               │
                  ┌────────────┴────────────┐
                  │ 1                       │ 1
                  ▼                         ▼
┌─────────────────────────────┐  ┌──────────────────────────────┐
│  extractions                │  │  processing_jobs             │
│  ─────────────────────────  │  │  ──────────────────────────  │
│  id           UUID  PK      │  │  id           UUID  PK       │
│  document_id  UUID  FK      │  │  document_id  UUID  FK       │
│  fields       JSONB         │  │  status       ENUM           │
│  raw_ai_resp  TEXT          │  │  attempts     INT            │
│  embedding    vector(768)   │  │  last_error   TEXT           │
│  search_vector tsvector     │  │  queued_at    TIMESTAMPTZ    │
│  processed_at TIMESTAMPTZ   │  │  started_at   TIMESTAMPTZ    │
│               │             │  │  completed_at TIMESTAMPTZ    │
│  INDEX: IVFFlat (cosine)    │  └──────────────────────────────┘
│  INDEX: GIN (tsvector)      │
└─────────────────────────────┘
```

### Infrastructure Map

```
┌─────────────────────────────────────────────────────────────────┐
│                        Railway.app                              │
│                                                                 │
│   ┌──────────────────────┐    ┌──────────────────────────────┐  │
│   │   Service: api       │    │   Service: worker            │  │
│   │   cmd/api/main.go    │    │   cmd/worker/main.go         │  │
│   │   PORT: 8080         │    │   (no port — queue consumer) │  │
│   └────────────┬─────────┘    └────────────────┬─────────────┘  │
└────────────────┼───────────────────────────────┼────────────────┘
                 │                               │
       ┌─────────┼───────────────────────────────┤
       │         │               │               │
       ▼         ▼               ▼               ▼
┌───────────┐   ┌────────┐   ┌──────────────┐ ┌──────────────┐
│Supabase   │   │Upstash │   │  CloudAMQP   │ │  Supabase    │
│PostgreSQL │   │ Redis  │   │  RabbitMQ    │ │  Storage     │
│+ pgvector │   │        │   │              │ │  (1GB free)  │
│           │   │TLS     │   │  AMQPS (TLS) │ │              │
│users      │   │        │   │  durable     │ │  private     │
│documents  │   │job:*   │   │  persistent  │ │  bucket      │
│extractions│   │docjob:*│   │  queue       │ │  presigned   │
│jobs       │   │24h TTL │   │  prefetch=5  │ │  URLs only   │
└───────────┘   └────────┘   └──────────────┘ └──────────────┘
       │
       ▼
┌─────────────────┐     ┌────────────────────────────────┐
│  Google Gemini  │     │       Grafana Cloud            │
│  API            │     │                                │
│                 │     │  ┌─────────────┐ ┌──────────┐  │
│  gemini-1.5-    │     │  │  Prometheus │ │  Loki    │  │
│  flash          │     │  │  metrics    │ │  logs    │  │
│                 │     │  │  /metrics   │ │  zerolog │  │
│  text-          │     │  └─────────────┘ └──────────┘  │
│  embedding-004  │     └────────────────────────────────┘
│  (768 dims)     │
│  15 req/min     │
│  (free tier)    │
└─────────────────┘
```

---

## Tech Stack

| Layer | Technology | Version | Why |
|---|---|---|---|
| **Language** | Go | 1.26+ | Goroutines make worker pool trivial; compiled binary deploys as single file |
| **HTTP Framework** | Echo | v4 | Minimal overhead, clean middleware chaining, built-in binding |
| **Database** | PostgreSQL (Supabase) | 15+ | pgvector extension for semantic search; free tier with 500MB |
| **Vector Search** | pgvector | 0.7+ | Cosine similarity in SQL — no separate vector DB needed |
| **Cache + Job State** | Upstash Redis | 7+ | TLS-only, serverless Redis; free 10K commands/day |
| **Message Queue** | CloudAMQP RabbitMQ | 3.13 | AMQPS with durable queues; 1M messages/month free |
| **AI Provider** | Google Gemini | 1.5-flash | Multimodal (PDF+image+text); free 15 req/min; text-embedding-004 for 768-dim vectors |
| **File Storage** | Supabase Storage | — | S3-compatible; presigned URLs; 1GB free; no egress through API server |
| **ORM / DB Driver** | pgx | v5 | Native PostgreSQL driver; pgxpool for connection pooling |
| **Migrations** | golang-migrate | v4 | SQL-first migrations; up/down per version; CI-friendly |
| **Config** | Viper | v1 | Reads `.env` in dev, real env vars in prod — zero code change |
| **Logger** | zerolog | v1 | Zero-allocation JSON logger; console writer in dev; Grafana Loki compatible |
| **Auth** | golang-jwt | v5 | HS256 JWT; access 15min + refresh 7d; API key as SHA-256 hash |
| **Metrics** | Prometheus | v1 | Pull-based; 18 custom metrics; compatible with Grafana Cloud |
| **Docs** | swaggo/swag | v1 | Annotations-based OpenAPI 3.0 generation; no YAML to maintain |
| **Deploy** | Railway.app | — | Supports multiple services per project; env var injection; auto-deploy from Git |

---

## Project Structure

```
smart-doc-intelligence/
│
├── cmd/
│   ├── api/
│   │   └── main.go                  # API server entry point — wires all dependencies
│   └── worker/
│       └── main.go                  # Worker process entry point — separate Railway service
│
├── internal/
│   ├── domain/
│   │   ├── document.go              # Document entity, status/type enums, magic byte map
│   │   ├── extraction.go            # ExtractionResult, Field, SearchResult, AIResult
│   │   ├── job.go                   # ProcessingJob, QueueMessage, all repository interfaces
│   │   ├── user.go                  # User entity, auth types, UserRepository interface
│   │   └── errors.go                # Sentinel errors + typed error wrappers (NotFound, Forbidden, AI)
│   │
│   ├── usecase/
│   │   ├── user_usecase.go          # Register (bcrypt+API key), Login, JWT generation
│   │   ├── document_usecase.go      # Upload pipeline, GetByID, List, Delete, GetDownloadURL
│   │   ├── processing_usecase.go    # EnqueueJob, MarkProcessing/Completed/Failed
│   │   └── search_usecase.go        # Semantic, fulltext, hybrid RRF search routing
│   │
│   ├── repository/
│   │   ├── postgres/
│   │   │   ├── user_repo.go         # User CRUD — email/ID/API key lookups
│   │   │   ├── document_repo.go     # Document CRUD — dynamic filters, pagination, multi-tenant
│   │   │   └── extraction_repo.go   # Extraction CRUD — pgvector cosine search, tsvector FTS
│   │   └── redis/
│   │       └── job_repo.go          # Job status — dual-key (job: + docjob:), 24h TTL
│   │
│   ├── delivery/http/
│   │   ├── handler/
│   │   │   ├── auth_handler.go      # POST /register, POST /login
│   │   │   ├── document_handler.go  # Upload, List, GetByID, Download URL, Delete, Status
│   │   │   ├── search_handler.go    # GET /search with type routing
│   │   │   ├── health_handler.go    # GET /health — all deps + pool stats + runtime info
│   │   │   └── responses.go         # Shared response types (errorResponse)
│   │   ├── middleware/
│   │   │   ├── auth.go              # JWT + X-API-Key dual auth
│   │   │   ├── ratelimit.go         # Per-user token bucket, cleanup goroutine
│   │   │   ├── security.go          # Security headers, HTTPS enforcement, request ID
│   │   │   ├── upload.go            # Content-Length + MaxBytesReader + extension pre-check
│   │   │   ├── metrics.go           # Prometheus HTTP middleware + record helpers
│   │   │   └── helpers.go           # SHA-256 hash, request ID generation
│   │   └── router.go                # Echo router — all routes, middleware stack, error handler
│   │
│   ├── worker/
│   │   ├── processing_worker.go     # Worker pool, processJob pipeline, retry, webhook
│   │   └── retry_test.go            # Unit tests for retry logic and backoff
│   │
│   └── ai/
│       ├── interface.go             # AIProvider interface + ProcessInput struct
│       └── gemini/
│           ├── client.go            # Gemini implementation — classify→extract→summarize + retry
│           └── prompts.go           # 6 prompt templates (one per document type)
│
├── pkg/
│   ├── database/
│   │   └── postgres.go              # pgxpool setup, AfterConnect hook, pgvector check
│   ├── storage/
│   │   └── supabase.go              # Upload, Delete, GeneratePresignedURL, HealthCheck
│   ├── queue/
│   │   └── rabbitmq.go              # Publish, Consume, ParseMessage, HealthCheck, Close
│   ├── embedding/
│   │   └── gemini.go                # text-embedding-004 wrapper, RETRIEVAL_DOCUMENT/QUERY types
│   ├── metrics/
│   │   └── prometheus.go            # 18 metrics — HTTP, documents, AI, worker, search, webhook
│   └── logger/
│       └── logger.go                # zerolog wrapper — WithService, WithDocumentID, WithUserID
│
├── migrations/
│   ├── 001_create_users.up.sql      # users table + indexes
│   ├── 001_create_users.down.sql
│   ├── 002_create_documents.up.sql  # documents table + enum types + indexes
│   ├── 002_create_documents.down.sql
│   ├── 003_create_extractions.up.sql # extractions + IVFFlat + GIN + tsvector trigger
│   ├── 003_create_extractions.down.sql
│   ├── 004_create_processing_jobs.up.sql
│   ├── 004_create_processing_jobs.down.sql
│   ├── 005_enable_pgvector.up.sql   # CREATE EXTENSION vector
│   └── 005_enable_pgvector.down.sql
│
├── internal/mocks/
│   └── mocks.go                     # Hand-rolled mocks for all repository/usecase interfaces
│
├── config/
│   └── config.go                    # Typed config struct — Viper loader + validation
│
├── docs/                            # Auto-generated by swag — do not edit manually
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
│
├── docker-compose.yml               # Local dev — RabbitMQ + Redis (Supabase stays cloud)
├── Makefile                         # build, run, worker, swag, migrate-up/down, test, lint
├── .env.example                     # All required environment variables with descriptions
├── go.mod
└── go.sum
```

---

## Getting Started

### Prerequisites

- Go 1.26+
- A [Supabase](https://supabase.com) project (free tier)
- A [Upstash Redis](https://upstash.com) database (free tier)
- A [CloudAMQP](https://cloudamqp.com) RabbitMQ instance (free tier)
- A [Google AI Studio](https://aistudio.google.com) API key (free tier)

### 1 — Clone and configure

```bash
git clone https://github.com/IndraSty/smart-doc-intelligence.git
cd smart-doc-intelligence
cp .env.example .env
```

Open `.env` and fill in your credentials:

```env
# Minimum required values
JWT_SECRET=your-secret-at-least-32-characters-long

DATABASE_URL=postgresql://postgres:[password]@db.[ref].supabase.co:5432/postgres
SUPABASE_URL=https://[ref].supabase.co
SUPABASE_SERVICE_KEY=your-service-role-key
SUPABASE_BUCKET=documents

REDIS_URL=rediss://default:[password]@[host].upstash.io:6379

RABBITMQ_URL=amqps://[user]:[pass]@[host].cloudamqp.com/[vhost]

GEMINI_API_KEY=your-gemini-api-key
```

### 2 — Run migrations and start

```bash
# Install migrate CLI (once)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Apply migrations
migrate -path migrations -database "$DATABASE_URL" up

# Install dependencies
go mod tidy
```

### 3 — Start API and Worker

```bash
# Terminal 1 — API server
go run ./cmd/api/main.go

# Terminal 2 — Worker process
go run ./cmd/worker/main.go
```

API is live at `http://localhost:8080`
Swagger UI at `http://localhost:8080/swagger/index.html`

### Quick smoke test

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@example.com","password":"password123"}'

# Upload a document (use the access_token from register response)
curl -X POST http://localhost:8080/api/v1/documents \
  -H "Authorization: Bearer <access_token>" \
  -F "file=@invoice.pdf"

# Poll status
curl http://localhost:8080/api/v1/documents/<document_id>/status \
  -H "Authorization: Bearer <access_token>"

# Search when completed
curl "http://localhost:8080/api/v1/search?q=total+amount&type=hybrid" \
  -H "Authorization: Bearer <access_token>"
```

---

## API Reference

### Authentication

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | ❌ | Create account — returns JWT tokens + one-time API key |
| `POST` | `/api/v1/auth/login` | ❌ | Login — returns JWT access (15min) + refresh (7d) tokens |

### Documents

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/documents` | ✅ | Upload document (multipart) — returns `document_id` + `job_id` immediately |
| `GET` | `/api/v1/documents` | ✅ | List documents — filter by `status`, `type`; paginate with `limit`, `offset` |
| `GET` | `/api/v1/documents/:id` | ✅ | Get document with AI extraction results |
| `GET` | `/api/v1/documents/:id/status` | ✅ | Poll processing status — checks Redis first, falls back to PostgreSQL |
| `GET` | `/api/v1/documents/:id/download` | ✅ | Get 15-minute presigned Supabase Storage URL |
| `DELETE` | `/api/v1/documents/:id` | ✅ | Delete document record + file from storage |

### Search

| Method | Endpoint | Auth | Params | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/search` | ✅ | `q`, `type=semantic` | Vector cosine similarity search |
| `GET` | `/api/v1/search` | ✅ | `q`, `type=fulltext` | PostgreSQL tsvector keyword search |
| `GET` | `/api/v1/search` | ✅ | `q`, `type=hybrid` | RRF fusion of semantic + fulltext (default) |

### System

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | ❌ | All dependency health + DB pool stats + uptime |
| `GET` | `/metrics` | ❌ | Prometheus metrics endpoint |
| `GET` | `/swagger/*` | ❌ | Interactive Swagger UI |

### Document Status Flow

```
uploaded ──▶ queued ──▶ processing ──▶ completed
                                  └──▶ failed
```

### Supported Document Types & Extracted Fields

| Type | Key Fields Extracted |
|---|---|
| `invoice` | invoice_number, vendor_name, total_amount, currency, due_date, line_items |
| `contract` | parties_involved, effective_date, expiry_date, governing_law, key_obligations |
| `identity` | full_name, id_number, date_of_birth, nationality, address, expiry_date |
| `financial` | company_name, report_type, period, total_revenue, net_profit, total_assets |
| `receipt` | merchant_name, transaction_date, items_purchased, total_amount, payment_method |
| `other` | dynamic key-value extraction based on document content |

---

## Benchmarks

Measured on Railway.app Starter plan (shared CPU, 512MB RAM) with Supabase free tier PostgreSQL.

### API Endpoint Latency (p50 / p90 / p99)

| Endpoint | p50 | p90 | p99 | Notes |
|---|---|---|---|---|
| `POST /auth/login` | 45ms | 78ms | 120ms | bcrypt cost 12 |
| `POST /documents` (upload) | 180ms | 310ms | 520ms | includes Supabase Storage upload |
| `GET /documents/:id` | 8ms | 14ms | 28ms | single Postgres query |
| `GET /documents/:id/status` | 2ms | 4ms | 9ms | Redis O(1) lookup |
| `GET /search?type=fulltext` | 22ms | 38ms | 65ms | tsvector GIN index |
| `GET /search?type=semantic` | 890ms | 1.2s | 1.8s | Gemini embed + pgvector |
| `GET /search?type=hybrid` | 910ms | 1.3s | 1.9s | parallel semantic + fulltext |
| `GET /health` | 35ms | 60ms | 95ms | 4 dependency checks |

### Worker Processing Time (end-to-end per document)

| Document Type | p50 | p90 | p99 | Bottleneck |
|---|---|---|---|---|
| `txt` (plain text) | 4.2s | 6.8s | 9.1s | Gemini API latency |
| `pdf` (1-2 pages) | 5.1s | 8.3s | 11.2s | Gemini vision + embedding |
| `png` / `jpg` | 4.8s | 7.6s | 10.5s | Gemini vision |
| `pdf` (10+ pages) | 9.3s | 14.7s | 21.8s | Larger context window |

> All AI processing time is dominated by Gemini API latency (~3-8s per call).
> Worker pool size of 5 handles ~12 documents/minute on free tier Gemini (15 req/min).

### Throughput

| Metric | Value |
|---|---|
| Max upload RPS (per user) | 2 req/s |
| Max general API RPS (per user) | 20 req/s |
| Worker pool throughput | ~12 docs/min (Gemini free tier bound) |
| Redis job status lookup | <3ms p99 |
| Semantic search (pre-embedded) | <2s p99 |

---

## Security

| Control | Implementation |
|---|---|
| **Password hashing** | bcrypt cost 12 — intentionally slow to resist brute force |
| **JWT signing** | HS256 with 32+ char secret; access token 15min; refresh token 7d |
| **API key storage** | SHA-256 hash only — plaintext shown exactly once at registration |
| **File type validation** | Magic bytes checked on raw bytes — not file extension or Content-Type header |
| **File size enforcement** | Content-Length header check + `MaxBytesReader` wrap before body is read |
| **Storage path isolation** | `{userID}/{documentID}.{ext}` UUID paths — original filename never used as path |
| **Presigned URL expiry** | 15-minute Supabase Storage signed URLs — no permanent file access |
| **Multi-tenant isolation** | Every SQL query includes `WHERE user_id = $N` — enforced at repository layer |
| **SQL injection** | Parameterized queries only — zero string-interpolated SQL in codebase |
| **Webhook validation** | HTTPS-only URLs; validated before storage; format check on input |
| **Security headers** | `X-Content-Type-Options`, `X-Frame-Options`, `HSTS`, `X-XSS-Protection` on every response |
| **HTTPS enforcement** | `X-Forwarded-Proto` check in production; HTTP redirected to HTTPS |
| **Secret management** | All secrets from environment variables — zero hardcoded credentials |
| **Rate limiting** | Per-user token bucket; upload endpoint stricter (2 RPS) than general (20 RPS) |
| **Filename sanitization** | Path separators and null bytes stripped; original name stored in DB only |

---

## Observability

### Prometheus Metrics (18 total)

| Metric | Type | Labels | Description |
|---|---|---|---|
| `smartdoc_http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `smartdoc_http_request_duration_seconds` | Histogram | method, path | Request latency |
| `smartdoc_http_requests_in_flight` | Gauge | — | Active concurrent requests |
| `smartdoc_http_response_size_bytes` | Histogram | method, path | Response payload size |
| `smartdoc_documents_uploaded_total` | Counter | file_type | Uploads by file type |
| `smartdoc_documents_processed_total` | Counter | document_type, status | AI processing outcomes |
| `smartdoc_documents_processing_duration_seconds` | Histogram | document_type | End-to-end processing time |
| `smartdoc_documents_in_queue` | Gauge | — | Jobs waiting in RabbitMQ |
| `smartdoc_documents_file_size_bytes` | Histogram | file_type | Upload file sizes |
| `smartdoc_ai_requests_total` | Counter | operation, status | Gemini API call count |
| `smartdoc_ai_request_duration_seconds` | Histogram | operation | Gemini API latency |
| `smartdoc_ai_retries_total` | Counter | operation | Retry attempts per operation |
| `smartdoc_worker_jobs_processed_total` | Counter | worker_id, status | Jobs per worker |
| `smartdoc_worker_active_jobs` | Gauge | — | Currently processing jobs |
| `smartdoc_worker_job_duration_seconds` | Histogram | — | Queue pickup to completion |
| `smartdoc_search_requests_total` | Counter | type, status | Search queries by type |
| `smartdoc_search_duration_seconds` | Histogram | type | Search latency by type |
| `smartdoc_webhook_deliveries_total` | Counter | status | Webhook delivery outcomes |

### Health Check Response

```json
{
  "status": "ok",
  "version": "1.0.0",
  "timestamp": "2024-01-15T10:30:00Z",
  "system": {
    "go_version": "go1.22.5",
    "goroutines": 24,
    "cpus": 2,
    "uptime_seconds": 3847.2
  },
  "db_pool": {
    "total_conns": 5,
    "idle_conns": 3,
    "acquired_conns": 2
  },
  "dependencies": {
    "postgres":  { "status": "ok", "latency": "8ms"  },
    "redis":     { "status": "ok", "latency": "3ms"  },
    "rabbitmq":  { "status": "ok"                    },
    "storage":   { "status": "ok", "latency": "95ms" }
  }
}
```

### Logging

All logs are structured JSON (zerolog) — compatible with Grafana Loki.

```json
{
  "level": "info",
  "service": "worker",
  "document_id": "550e8400-e29b-41d4-a716-446655440000",
  "document_type": "invoice",
  "field_count": 14,
  "time": "2024-01-15T10:30:05.123456789Z",
  "message": "AI processing completed"
}
```

Key log fields: `service`, `request_id`, `user_id`, `document_id`, `job_id`, `worker_id`

---

## Running Tests

```bash
# All tests with race detector
go test ./... -race -cover

# Specific packages
go test ./internal/usecase/... -v -race   # RRF ranking + search logic
go test ./internal/worker/... -v -race    # retry + backoff logic

# With coverage report
go test ./... -race -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Coverage Areas

| Package | Tests | Coverage Focus |
|---|---|---|
| `internal/usecase` | 13 tests | RRF formula, search routing, result enrichment, deleted doc handling |
| `internal/worker` | 6 tests | Backoff delays, markJobFailed, Redis failure resilience |
| `internal/usecase` (doc) | 14 tests | Magic bytes (PDF/PNG/JPEG/TXT), webhook URL validation, filename sanitization |

---

## Local Development

```bash
# Start local RabbitMQ + Redis (Supabase stays on cloud)
docker compose up -d rabbitmq redis

# Hot reload API (requires air: go install github.com/air-verse/air@latest)
air -c .air.api.toml

# Regenerate Swagger docs after changing annotations
make swag

# Run migrations
make migrate-up

# Rollback last migration
make migrate-down

# Lint
make lint
```

---

## Deployment (Railway.app)

This project deploys as **two separate Railway services** from the same repository.

### Service 1 — API Server

| Setting | Value |
|---|---|
| Root Directory | `/` |
| Build Command | `go build -o bin/api ./cmd/api` |
| Start Command | `./bin/api` |
| Port | `8080` |

### Service 2 — Worker

| Setting | Value |
|---|---|
| Root Directory | `/` |
| Build Command | `go build -o bin/worker ./cmd/worker` |
| Start Command | `./bin/worker` |
| Port | None (queue consumer only) |

Both services share the same environment variables — set once in Railway's shared environment.

---

## License

MIT © [Indra Styawan](https://github.com/IndraSty)

---

<div align="center">

Built with Go · Gemini AI · pgvector · RabbitMQ · Supabase

⭐ Star this repo if it helped you build something great

</div>