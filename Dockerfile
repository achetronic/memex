# ─── Stage 1: Build Vue frontend ─────────────────────────────────────────────
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build


# ─── Stage 2: Build Go binary ─────────────────────────────────────────────────
FROM golang:1.24-alpine AS go-builder

ARG VERSION=dev

# Install build dependencies.
RUN apk add --no-cache git

WORKDIR /app/server

# Cache Go module downloads before copying source.
COPY server/go.mod server/go.sum ./
RUN go mod download

# Copy source and built frontend into the embed path expected by go:embed.
COPY server/ ./
COPY --from=frontend-builder /app/frontend/dist ./cmd/frontend_dist

# Generate swagger docs into docs/api/ to match the import path in router.go.
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init -g cmd/main.go -o docs/api/

# Build a fully static binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /memex ./cmd/


# ─── Stage 3: Minimal production image ───────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=go-builder /memex /memex

# Expose the default HTTP port.
EXPOSE 8080

ENTRYPOINT ["/memex"]
