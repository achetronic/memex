# memex

Self-hosted, generic RAG (Retrieval-Augmented Generation) backend powered by
PostgreSQL + pgvector and Ollama. Upload documents via a web UI or REST API,
index them as vector embeddings, and query them semantically.

---

## Features

- **Multi-format ingestion**: PDF, TXT, Markdown, DOCX, ODT, HTML, CSV, JSON, YAML, TOML, XML, RTF, EML
- **Semantic search**: cosine similarity via pgvector, powered by Ollama embeddings
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

# Set your Ollama URL if it's not on localhost
export OLLAMA_URL=http://your-ollama-host:11434

# Start everything
docker compose up -d
```

Open http://localhost:8080 for the UI or http://localhost:8080/swagger/index.html for the API docs.

---

## Configuration

All configuration is done via environment variables. The docker-compose.yml passes
them through from your shell, so you can set them in a `.env` file or export them.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `LOG_FORMAT` | `json` | `console` or `json` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `DATABASE_URL` | *(required)* | PostgreSQL DSN (set automatically in docker-compose) |
| `OPENAI_BASE_URL` | `http://host.docker.internal:11434` | Base URL of any OpenAI-compatible API (Ollama, OpenAI, Groq…) |
| `OPENAI_API_KEY` | `ollama` | API key — use any non-empty string for Ollama, real key for OpenAI |
| `OPENAI_EMBEDDING_MODEL` | `nomic-embed-text` | Embedding model name — must be available in the provider |
| `OPENAI_EMBEDDING_DIM` | `768` | Vector dimensions — must match the model output |
| `WORKER_POOL_SIZE` | `3` | Concurrent ingestion workers |
| `WORKER_MAX_RETRIES` | `3` | Max attempts per document before marking as failed |
| `CHUNK_SIZE` | `512` | Target chunk size in words |
| `CHUNK_OVERLAP` | `64` | Overlap between consecutive chunks in words |
| `SEARCH_DEFAULT_LIMIT` | `5` | Default number of results returned by search |
| `MAX_UPLOAD_SIZE_MB` | `50` | Maximum upload file size |

---

## API Reference

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
| `GET` | `/api/v1/health` | Returns status of database and Ollama |

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

memex is designed to be used as a knowledge base by AI agents via MCP servers.
A companion `memex-mcp` can expose these tools:

- `search_knowledge_base(query, limit)` → calls `POST /api/v1/search`
- `upload_document(path)` → calls `POST /api/v1/documents`
- `list_documents(status)` → calls `GET /api/v1/documents`

---

## Development

### Prerequisites

- Go 1.22+
- Node 20+
- Docker + Docker Compose
- [swag](https://github.com/swaggo/swag): `go install github.com/swaggo/swag/cmd/swag@latest`
- Ollama with `nomic-embed-text` pulled: `ollama pull nomic-embed-text`

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
```

### Regenerate Swagger docs

```bash
cd server
swag init -g cmd/main.go -o docs/
```

---

## Release

Releases are automated via GitHub Actions:

- **Tag a version** (`git tag v1.0.0 && git push --tags`) to trigger:
  - Binary builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
  - Docker multi-arch image pushed to `ghcr.io/achetronic/memex`

---

## License

MIT
