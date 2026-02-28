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

All configuration lives in a single YAML file. Pass it with `-config`:

```bash
memex -config /path/to/config.yaml
```

If `-config` is not specified, memex looks for `config.yaml` in the working directory. If the file is not found, it exits with an error.

All string values support `${ENV_VAR}` expansion — use it for secrets and environment-specific values without writing them in plain text.

A fully documented example is in [`server/docs/config.yaml`](server/docs/config.yaml). The main sections are:

| Section | Description |
|---|---|
| `server` | HTTP port |
| `log` | Format (`console`/`json`) and level (`debug`, `info`, `warn`, `error`) |
| `database` | PostgreSQL DSN |
| `embeddings` | Base URL, API key, model name and dimensions |
| `worker` | Pool size and max retries |
| `chunker` | Chunk size and overlap (in words) |
| `search` | Default result limit |
| `upload` | Max file size in MB |
| `namespaces` | Declared namespaces (requests to undeclared ones → 400) |
| `auth.api_keys` | API keys and their allowed namespaces (empty → auth disabled) |

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
swag init -g cmd/main.go -o docs/api/
go run ./cmd/ -config docs/config.yaml
```

### Regenerate Swagger docs

```bash
cd server
swag init -g cmd/main.go -o docs/api/
```

---

## Release

Releases are automated via GitHub Actions. To publish a new release:

1. Go to [Releases](https://github.com/achetronic/memex/releases/new) and create a new release with the desired tag (e.g. `v1.0.0`).
2. Publishing the release triggers:
   - Binary builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
   - Docker multi-arch image pushed to `ghcr.io/achetronic/memex`
   - `memex-mcp` binaries and Docker image (`ghcr.io/achetronic/memex/memex-mcp`) published alongside

---

## License

Apache 2.0 — see [LICENSE](LICENSE)
