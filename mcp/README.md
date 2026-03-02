# memex-mcp

MCP server for [Memex](https://github.com/achetronic/memex) — expose your
self-hosted RAG knowledge base as a set of tools for any AI agent that speaks
the Model Context Protocol.

---

## Table of contents

1. [Overview](#overview)
2. [Tools](#tools)
3. [Transport modes](#transport-modes)
   - [stdio](#stdio)
   - [HTTP (Streamable HTTP)](#http-streamable-http)
4. [Configuration reference](#configuration-reference)
   - [server](#server)
   - [memex](#memex)
   - [memex.auth — API key resolution](#memexauth--api-key-resolution)
   - [middleware.access_logs](#middlewareaccess_logs)
   - [middleware.jwt](#middlewarejwt)
   - [policies.rules](#policiesrules)
   - [oauth_authorization_server](#oauth_authorization_server)
   - [oauth_protected_resource](#oauth_protected_resource)
5. [Running memex-mcp](#running-memex-mcp)
   - [Binary](#binary)
   - [Docker](#docker)
   - [Claude Desktop](#claude-desktop)
6. [Namespaces](#namespaces)
7. [API key authentication](#api-key-authentication)
8. [JWT auth and policies](#jwt-auth-and-policies)

---

## Overview

memex-mcp sits between your AI agents and your Memex instance. It translates
MCP tool calls into Memex REST API requests, handles namespace routing via the
`X-Memex-Namespace` header, and optionally enforces JWT-based access control
and per-namespace API key resolution.

```
Agent ──MCP──▶ memex-mcp ──HTTP──▶ Memex API ──▶ PostgreSQL + pgvector
```

---

## Tools

| Tool | Description |
|---|---|
| `search` | Semantic search over documents in a namespace |
| `list_documents` | List documents, optionally filtered by ingestion status |
| `get_document` | Get detail and ingestion status of a single document |
| `upload_document` | Upload a local file to Memex for ingestion |
| `delete_document` | Delete a document and all its chunks |
| `health` | Check connectivity of the upstream Memex instance |

All tools except `health` accept an optional `namespace` parameter. When
omitted, the `memex.default_namespace` from the config is used. If that is
also empty, no namespace header is sent.

---

## Transport modes

### stdio

The agent launches memex-mcp as a subprocess and communicates over stdin/stdout.
No network port is opened. Ideal for local integrations (Claude Desktop, Cursor,
VS Code extensions).

```bash
memex-mcp -config config.yaml
```

Config `server.transport.type` must be `"stdio"` (or omitted — stdio is the default).

### HTTP (Streamable HTTP)

memex-mcp listens on a TCP port and exposes a single `/mcp` endpoint using the
[Streamable HTTP transport](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#streamable-http).
Suitable for multi-agent or multi-tenant deployments where agents connect over
the network.

```bash
memex-mcp -config config.yaml
# Listening on :8090/mcp
```

Config `server.transport.type` must be `"http"`.

---

## Configuration reference

All configuration is in a YAML file passed via `-config <path>`.
Environment variables in the form `${VAR}` are expanded at load time.

### server

```yaml
server:
  name: "memex-mcp"
  version: "0.1.0"
  transport:
    type: "stdio"       # "stdio" or "http"
    http:
      host: ":8090"     # only used when type is "http"
```

### memex

```yaml
memex:
  base_url: "http://localhost:8080"   # required — root URL of the Memex API
  default_namespace: ""               # optional — used when no namespace is passed per-call
```

### memex.auth — API key resolution

When the Memex API requires authentication, memex-mcp resolves the API key for
each request using this precedence (first non-empty value wins):

1. **Forwarded header** — when `forward_api_key: true`, the agent sends
   `X-Memex-Api-Key` to memex-mcp and it is forwarded verbatim to Memex.
   **JWT auth must be enabled** when using this option — without it any
   unauthenticated caller could inject an arbitrary key.
2. **Static key for the namespace** — looked up in `namespace_keys`.
3. **Static key for `"*"`** — catch-all fallback in `namespace_keys`.
4. **No credential** — compatible with Memex instances that have no auth yet.

```yaml
memex:
  base_url: "http://localhost:8080"
  auth:
    forward_api_key: true              # agents send X-Memex-Api-Key; JWT auth required
    namespace_keys:
      invoices:  "${MEMEX_KEY_INVOICES}"
      contracts: "${MEMEX_KEY_CONTRACTS}"
      "*":       "${MEMEX_KEY_DEFAULT}"  # catch-all
```

### middleware.access_logs

```yaml
middleware:
  access_logs:
    excluded_headers:
      - "Accept"
      - "Accept-Encoding"
    redacted_headers:
      - "Authorization"
      - "Cookie"
```

### middleware.jwt

Only applies to HTTP transport. When `enabled: false` (or the section is
omitted), all requests are allowed through.

```yaml
middleware:
  jwt:
    enabled: true
    jwks_uri: "https://your-idp.com/.well-known/jwks.json"
    cache_interval: 5m
    allow_conditions:
      - expression: 'has(payload.scope) && payload.scope.contains("memex:read")'
```

`allow_conditions` are [CEL](https://cel.dev) expressions evaluated against the
decoded JWT payload. A request is allowed if **any** condition evaluates to true.

### policies.rules

Rules unify tool and namespace access control into a single list. Rules are
evaluated in order — the first whose CEL expression matches wins. Within the
matching rule, `allowed_tools` and `allowed_namespaces` are both enforced with
AND semantics: the tool must be allowed **and** the namespace must be allowed.

An empty `allowed_tools` or `allowed_namespaces` means no restriction on that
dimension. Use `"*"` to explicitly allow everything.

`allowed_tools` supports exact names and prefix wildcards (`"search_*"`).

```yaml
policies:
  rules:
    # Admins can use all tools across all namespaces.
    - expression: 'has(payload.groups) && payload.groups.exists(g, g == "admins")'
      allowed_tools: ["*"]
      allowed_namespaces: ["*"]

    # Write agents: restricted tools, own namespace only.
    - expression: 'has(payload.scope) && payload.scope.contains("memex:write") && has(payload.sub)'
      allowed_tools: ["upload_document", "delete_document", "list_documents", "get_document", "search", "health"]
      allowed_namespaces: ["${payload.sub}"]

    # Read agents: read-only tools, own namespace only.
    - expression: 'has(payload.scope) && payload.scope.contains("memex:read") && has(payload.sub)'
      allowed_tools: ["list_documents", "get_document", "search", "health"]
      allowed_namespaces: ["${payload.sub}"]
```

### oauth_authorization_server

Serves `/.well-known/oauth-authorization-server` by proxying the issuer's
OpenID configuration. Required by some MCP clients for OAuth discovery.

```yaml
oauth_authorization_server:
  enabled: true
  issuer_uri: "https://your-idp.com"
```

### oauth_protected_resource

Serves `/.well-known/oauth-protected-resource` from the configured values.

```yaml
oauth_protected_resource:
  enabled: true
  resource: "https://your-memex-mcp.example.com"
  auth_servers:
    - "https://your-idp.com"
  jwks_uri: "https://your-idp.com/.well-known/jwks.json"
  scopes_supported: ["memex:read", "memex:write"]
  bearer_methods_supported: ["header"]
  resource_name: "Memex MCP Server"
  resource_documentation: "https://github.com/achetronic/memex"
```

---

## Running memex-mcp

### Binary

Download the binary for your platform from the [releases page](https://github.com/achetronic/memex/releases).

```bash
# stdio mode (default)
memex-mcp -config config.yaml

# HTTP mode
memex-mcp -config config-http.yaml
```

Example config files are in [`docs/`](docs/).

### Docker

```bash
docker run --rm \
  -v /path/to/config.yaml:/config/config.yaml \
  ghcr.io/achetronic/memex/memex-mcp:latest
```

For HTTP mode, expose the port:

```bash
docker run --rm \
  -p 8090:8090 \
  -v /path/to/config-http.yaml:/config/config.yaml \
  ghcr.io/achetronic/memex/memex-mcp:latest
```

### Claude Desktop

Add memex-mcp to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memex": {
      "command": "/path/to/memex-mcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

---

## Namespaces

Namespaces are logical partitions within a Memex instance. They are forwarded
as the `X-Memex-Namespace` HTTP header on every API request.

You can set a namespace per tool call:

```json
{ "tool": "search", "arguments": { "query": "invoice terms", "namespace": "invoices" } }
```

Or configure a default that applies when no namespace is supplied:

```yaml
memex:
  default_namespace: "invoices"
```

---

## API key authentication

memex-mcp resolves the Memex API key for each request in this order:

1. The `X-Memex-Api-Key` header from the agent's incoming HTTP request, when
   `memex.auth.forward_api_key: true`.
2. The static key configured in `memex.auth.namespace_keys` for the specific
   namespace.
3. The static key configured for `"*"` in `memex.auth.namespace_keys`.
4. No credential (no-auth Memex instances).

**`forward_api_key: true` requires JWT auth to be enabled.** This option lets
each agent supply its own key, but it depends entirely on the caller being who
they claim to be. Without JWT validation, any client can send an arbitrary
`X-Memex-Api-Key` and impersonate another agent. Enable `middleware.jwt` first.

---

## JWT auth and policies

JWT validation and policy enforcement only apply to the **HTTP transport**.
In stdio mode all calls are trusted — access control is delegated to the
operating system (only processes that can launch memex-mcp can use it).

Policies in `policies.rules` are evaluated only after the JWT middleware has
validated the token. The JWT check is a prerequisite — it runs first and
provides the `payload` variable that CEL expressions reference.

For a full HTTP + JWT + policies example see [`docs/config-http.yaml`](docs/config-http.yaml).
