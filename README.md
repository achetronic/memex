<div align="center">
  <img src="docs/images/header.svg" alt="Memex" width="860"/>
</div>

# Memex

Memex is a self-hosted RAG (Retrieval-Augmented Generation) system built on PostgreSQL + pgvector.
Feed it your documents, query them in plain language, and plug the results into any AI agent or workflow.
No cloud dependency, no vendor lock-in — just your data, your server, and an OpenAI-compatible embeddings API (Ollama, OpenAI, Groq…).

---

## Features

- **Multi-format ingestion**: PDF, TXT, Markdown, DOCX, ODT, HTML, CSV, JSON, YAML, TOML, XML, RTF, EML
- **Semantic search**: cosine similarity via pgvector, powered by any OpenAI-compatible embeddings API
- **Namespaces**: logical partitions — each document belongs to a namespace, queries are always scoped
- **API key auth**: optional per-namespace key validation, configured in a YAML file with env var expansion
- **Resilient worker**: configurable pool size, exponential backoff retries, graceful failure reporting
- **REST API**: fully documented with Swagger UI at `/swagger/index.html`
- **Vue 3 frontend**: upload, manage and search documents — served by the Go binary itself
- **Single docker-compose**: postgres (with pgvector) + memex, ready to run

---

## Quick Start

```bash
# Clone the repository
git clone https://github.com/achetronic/memex
cd memex

# Set your embeddings API base URL if it's not on localhost (e.g. Ollama)
export OPENAI_BASE_URL=http://your-ollama-host:11434

# Start everything
docker compose up -d
```

Open http://localhost:8080 for the UI or http://localhost:8080/swagger/index.html for the API docs.

---

## Configuration

### Environment variables

All runtime tunables are set via environment variables. The docker-compose.yml passes
them through from your shell, so you can set them in a `.env` file or export them.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `LOG_FORMAT` | `json` | `console` or `json` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `DATABASE_URL` | *(required)* | PostgreSQL DSN (set automatically in docker-compose) |
| `OPENAI_BASE_URL` | `http://localhost:11434` | Base URL of any OpenAI-compatible API (Ollama, OpenAI, Groq…) |
| `OPENAI_API_KEY` | `ollama` | API key — use any non-empty string for Ollama, real key for OpenAI |
| `OPENAI_EMBEDDING_MODEL` | `nomic-embed-text` | Embedding model name — must be available in the provider |
| `OPENAI_EMBEDDING_DIM` | `768` | Vector dimensions — must match the model output |
| `WORKER_POOL_SIZE` | `3` | Concurrent ingestion workers |
| `WORKER_MAX_RETRIES` | `3` | Max attempts per document before marking as failed |
| `CHUNK_SIZE` | `512` | Target chunk size in words |
| `CHUNK_OVERLAP` | `64` | Overlap between consecutive chunks in words |
| `SEARCH_DEFAULT_LIMIT` | `5` | Default number of results returned by search |
| `MAX_UPLOAD_SIZE_MB` | `50` | Maximum upload file size |

### Config file (namespaces and auth)

Namespaces and API key authentication are configured in an optional YAML file
passed via the `-config` flag:

```bash
memex -config /path/to/config.yaml
```

All string values in the file support `${ENV_VAR}` expansion, so secrets never
need to be written in plain text.

```yaml
namespaces:
  - name: invoices
  - name: contracts
  - name: general

auth:
  api_keys:
    - key: "${MEMEX_KEY_ADMIN}"
      namespaces: ["*"]          # access to all namespaces
    - key: "${MEMEX_KEY_SERVICE}"
      namespaces: ["invoices", "contracts"]
    - key: "${MEMEX_KEY_GENERAL}"
      namespaces: ["general"]
```

A full example is in [`server/docs/config.yaml`](server/docs/config.yaml).

When the file is not provided (or `auth.api_keys` is empty), auth is disabled
and all requests are allowed through without any key or namespace header —
useful for local and single-tenant deployments.

---

## Namespaces

Namespaces are logical partitions within a single Memex instance. Every
document belongs to a namespace and every query is scoped to one. They are
passed as HTTP headers on each request:

| Header | Description |
|---|---|
| `X-Memex-Namespace` | Target namespace for the operation |
| `X-Memex-Api-Key` | API key for authentication (when auth is enabled) |

When auth is disabled the headers are optional. When auth is enabled, both are
required on every `/api/v1/*` request.

Requests to an undeclared namespace are rejected with `400 Bad Request`.
Requests with a valid key but no access to the namespace are rejected with `403 Forbidden`.

---

## API Reference

All `/api/v1/*` endpoints accept these headers:

| Header | Required | Description |
|---|---|---|
| `X-Memex-Namespace` | When auth enabled | Namespace to operate on |
| `X-Memex-Api-Key` | When auth enabled | API key for authentication |

### Documents

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/documents` | Upload a document (multipart/form-data, field: `file`) |
| `GET` | `/api/v1/documents` | List documents. Optional `?status=` filter |
| `GET` | `/api/v1/documents/{id}` | Get document detail and ingestion status |
| `DELETE` | `/api/v1/documents/{id}` | Delete document and all its chunks |

### Search

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/search` | Semantic search |

Search request body:
```json
{
  "query": "your natural language question",
  "limit": 5
}
```

### Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Returns status of database and embeddings API |

---

## Document Status Flow

```
pending → processing → completed
                    ↘ failed
```

Documents in `failed` status have their error message stored and visible via
`GET /api/v1/documents/{id}`. They can be deleted and re-uploaded.

---

## MCP Integration

Memex ships with a companion MCP server — [`memex-mcp`](mcp/README.md) — that
exposes your knowledge base as tools for any AI agent that speaks the
[Model Context Protocol](https://modelcontextprotocol.io).

Available tools: `search`, `list_documents`, `get_document`, `upload_document`,
`delete_document`, `health`.

Supports **stdio** (Claude Desktop, Cursor, VS Code) and **HTTP** (multi-agent
deployments) transports, with optional JWT auth and per-rule CEL access policies.

See [`mcp/README.md`](mcp/README.md) for full documentation.

---

## Development

### Prerequisites

- Go 1.22+
- Node 20+
- Docker + Docker Compose
- [swag](https://github.com/swaggo/swag): `go install github.com/swaggo/swag/cmd/swag@latest`
- An OpenAI-compatible embeddings API (e.g. Ollama with `nomic-embed-text`: `ollama pull nomic-embed-text`)

### Run locally

```bash
# Start only postgres
docker compose up -d postgres

# Build and serve the frontend in watch mode
cd frontend && npm ci && npm run dev

# In another terminal, generate swagger docs and start the Go server
cd server
swag init -g cmd/main.go -o docs/
DATABASE_URL=postgres://memex:memex@localhost:5432/memex go run ./cmd/

# With namespaces and auth:
DATABASE_URL=postgres://memex:memex@localhost:5432/memex go run ./cmd/ -config docs/config.yaml
```

### Regenerate Swagger docs

```bash
cd server
swag init -g cmd/main.go -o docs/
```

---

## Release

Releases are automated via GitHub Actions. To publish a new release:

1. Go to [Releases](https://github.com/achetronic/memex/releases/new) and create a new release with the desired tag (e.g. `v1.0.0`).
2. Publishing the release triggers:
   - Binary builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
   - Docker multi-arch image pushed to `ghcr.io/achetronic/memex`
   - `memex-mcp` binaries and Docker image (`ghcr.io/achetronic/memex-mcp`) published alongside

---

## License

Apache 2.0 — see [LICENSE](LICENSE)
