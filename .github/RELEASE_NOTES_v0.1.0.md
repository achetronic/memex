# Memex v0.1.0

First public release. Memex is a self-hosted RAG (Retrieval-Augmented Generation) system
built on PostgreSQL + pgvector — your knowledge, elephant powered.

## What's included

**Core**
- REST API for document ingestion and semantic search, fully documented with Swagger UI
- Resilient ingestion worker: configurable pool size, exponential backoff retries, per-document failure reporting
- Semantic search via cosine similarity using pgvector
- Any OpenAI-compatible embeddings API supported out of the box (Ollama, OpenAI, Groq…)

**Document support**
- PDF, TXT, Markdown, DOCX, ODT, HTML, CSV, JSON, YAML, TOML, XML, RTF, EML

**Frontend**
- Vue 3 web UI for uploading, managing and searching documents
- Served directly by the Go binary — no separate web server needed

**Deployment**
- Single `docker compose up` gets you Postgres (with pgvector) + Memex running
- Multi-arch Docker image: `linux/amd64`, `linux/arm64`
- Available at `ghcr.io/achetronic/memex`

## Quick start

```bash
git clone https://github.com/achetronic/memex
cd memex
docker compose up -d
```

Open http://localhost:8080 for the UI or http://localhost:8080/swagger/index.html for the API docs.

## MCP Integration (roadmap)

A companion `memex-mcp` server is planned to expose Memex as a tool for AI agents,
with `search_knowledge_base`, `upload_document` and `list_documents` operations.

## Binaries

Pre-built binaries are available below for:

| Platform | Architecture |
|---|---|
| Linux | amd64, arm64 |
| macOS | amd64 (Intel), arm64 (Apple Silicon) |
| Windows | amd64 |

SHA-256 checksums are included in `checksums.txt`.
