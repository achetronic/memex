# Memex — Roadmap & Open Questions

This file tracks ideas, pending decisions and future work.
Nothing here is committed to any timeline.

---

## Multi-tenancy vs multi-instance

Right now Memex is single-tenant: all documents share the same pool and every
search returns results from the entire dataset. This limits its usefulness when
multiple agents or users need isolated knowledge bases.

Two approaches are being considered:

**Option A — Namespaces (multi-tenant, single instance)**
Add a `namespace` field to documents and chunks. Ingestion and search both
accept a namespace parameter, and pgvector filters by it alongside the
similarity query. Simple schema change, low operational cost. Still requires
some form of authentication (API keys at minimum) to prevent cross-namespace
reads.

**Option B — Multi-instance**
Each agent or tenant gets its own Memex deployment. Maximum isolation, no code
changes needed. Higher operational cost — works better in Kubernetes with a
Helm chart than with plain Docker Compose.

The right answer probably depends on the target use case:
- Personal / homelab → multi-instance is fine
- Shared platform or SaaS-style → namespaces make more sense

No decision made yet.

---

## MCP Integration

A companion MCP server to expose Memex as a tool for AI agents, with at least:

- `search_knowledge_base(query, limit)`
- `upload_document(path)`
- `list_documents(status)`

**Architecture decision:** the MCP server will live at `mcp/` in this repo,
as a separate Go module with its own binary. It will communicate with the
Memex REST API — no changes to the server are needed. Transport: Streamable
HTTP (modern MCP over HTTP + SSE).

Structure:
```
memex/
├── frontend/
├── server/
└── mcp/
```

---

## API key protection

The REST API is currently open with no authentication. Any client that can
reach the server can read and write all documents. Even a simple static API
key passed as a header would meaningfully improve this.

---
