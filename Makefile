# ─────────────────────────────────────────────────────────────────────────────
# memex Makefile
#
# Usage:
#   make dependencies   Install all required tools and fetch Go/Node modules
#   make run            Start postgres + run the Go server locally (dev mode)
#   make build          Build frontend + generate swagger + compile Go binary
#   make test           Run Go tests
#   make lint           Run golangci-lint
#   make swagger        (Re)generate Swagger docs from annotations
#   make docker-build   Build single-arch Docker image for current platform
#   make docker-push    Push the image built by docker-build
#   make docker-buildx  Build and push multi-arch image (linux/amd64+arm64)
#   make clean          Remove build artifacts
# ─────────────────────────────────────────────────────────────────────────────

# ── Variables ─────────────────────────────────────────────────────────────────

BINARY        := memex
DIST_DIR      := dist

REGISTRY      := ghcr.io
IMAGE_OWNER   := achetronic
IMAGE_NAME    := $(REGISTRY)/$(IMAGE_OWNER)/$(BINARY)
VERSION       := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE_TAG     := $(VERSION)

SERVER_DIR    := server
MAIN_PKG      := ./cmd/
DOCS_DIR      := $(SERVER_DIR)/docs

# The embed target dir — go:embed requires the path to be inside the package dir.
EMBED_DIR     := $(SERVER_DIR)/cmd/frontend_dist

CGO_ENABLED   := 0
LDFLAGS       := -s -w -X main.version=$(VERSION)

FRONTEND_DIR  := frontend
FRONTEND_DIST := $(FRONTEND_DIR)/dist

PLATFORMS     := linux/amd64,linux/arm64
COMPOSE_FILE  := docker-compose.yml

GOBIN         := $(shell go env GOPATH 2>/dev/null || echo $$HOME/go)/bin

# ── Phony targets ─────────────────────────────────────────────────────────────

.PHONY: dependencies run build test lint swagger \
        frontend-build swagger-gen embed-copy \
        docker-build docker-push docker-buildx \
        clean help

.DEFAULT_GOAL := help

# ── dependencies ──────────────────────────────────────────────────────────────

## Install all tools and fetch all Go/Node dependencies needed to build and run.
dependencies:
	@echo "==> Installing swag (Swagger doc generator)..."
	go install github.com/swaggo/swag/cmd/swag@latest

	@echo "==> Installing golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

	@echo "==> Fetching Go modules..."
	cd $(SERVER_DIR) && go mod download && go mod tidy

	@echo "==> Installing Node dependencies..."
	cd $(FRONTEND_DIR) && npm ci

	@echo "==> Generating initial Swagger docs..."
	$(GOBIN)/swag init -g cmd/main.go -d $(SERVER_DIR) -o $(DOCS_DIR)

	@echo "==> Creating placeholder embed dir for go:embed..."
	mkdir -p $(EMBED_DIR)
	@[ -f $(EMBED_DIR)/index.html ] || echo "<html><body>run make build</body></html>" > $(EMBED_DIR)/index.html

	@echo ""
	@echo "All dependencies ready. You can now run:"
	@echo "  make run    — start postgres and the Go server locally"
	@echo "  make build  — compile a production binary"

# ── run ───────────────────────────────────────────────────────────────────────

## Start postgres via docker compose and run the Go server in dev mode.
run: swagger-gen
	@echo "==> Starting postgres..."
	docker compose -f $(COMPOSE_FILE) up -d postgres

	@echo "==> Waiting for postgres to be healthy..."
	@until docker compose -f $(COMPOSE_FILE) exec postgres pg_isready -U memex > /dev/null 2>&1; do sleep 1; done

	@echo "==> Starting memex server (dev mode)..."
	cd $(SERVER_DIR) && \
	  DATABASE_URL=$${DATABASE_URL:-postgres://memex:memex@localhost:5432/memex} \
	  OPENAI_BASE_URL=$${OPENAI_BASE_URL:-http://localhost:11434} \
	  OPENAI_API_KEY=$${OPENAI_API_KEY:-ollama} \
	  OPENAI_EMBEDDING_MODEL=$${OPENAI_EMBEDDING_MODEL:-nomic-embed-text} \
	  LOG_FORMAT=$${LOG_FORMAT:-console} \
	  go run $(MAIN_PKG)

# ── build ─────────────────────────────────────────────────────────────────────

## Build frontend, copy dist for embed, generate swagger, compile the binary.
build: frontend-build embed-copy swagger-gen
	@echo "==> Compiling Go binary for $(shell go env GOOS)/$(shell go env GOARCH)..."
	@mkdir -p $(DIST_DIR)
	cd $(SERVER_DIR) && \
	  CGO_ENABLED=$(CGO_ENABLED) go build \
	    -ldflags="$(LDFLAGS)" \
	    -o ../$(DIST_DIR)/$(BINARY) \
	    $(MAIN_PKG)
	@echo "==> Binary ready: $(DIST_DIR)/$(BINARY)"

# ── frontend-build ────────────────────────────────────────────────────────────

## Build the Vue 3 frontend and output to frontend/dist/.
frontend-build:
	@echo "==> Building frontend..."
	cd $(FRONTEND_DIR) && npm run build

# ── embed-copy ────────────────────────────────────────────────────────────────

## Copy frontend/dist into server/cmd/frontend_dist for go:embed.
## go:embed cannot use paths that go outside the module directory (no ..).
embed-copy:
	@echo "==> Copying frontend dist into embed path..."
	rm -rf $(EMBED_DIR)
	cp -r $(FRONTEND_DIST) $(EMBED_DIR)

# ── swagger-gen ───────────────────────────────────────────────────────────────

## Generate/update Swagger docs from Go annotations.
swagger-gen:
	@echo "==> Generating Swagger docs..."
	$(GOBIN)/swag init -g cmd/main.go -d $(SERVER_DIR) -o $(DOCS_DIR)

swagger: swagger-gen

# ── test ──────────────────────────────────────────────────────────────────────

## Run all Go tests with race detector.
test:
	@echo "==> Running tests..."
	cd $(SERVER_DIR) && go test -race ./...

# ── lint ──────────────────────────────────────────────────────────────────────

## Run golangci-lint on the server code.
lint:
	@echo "==> Running linter..."
	$(GOBIN)/golangci-lint run $(SERVER_DIR)/...

# ── docker-build ──────────────────────────────────────────────────────────────

## Build a Docker image for the current platform (no push).
docker-build:
	@echo "==> Building Docker image $(IMAGE_NAME):$(IMAGE_TAG)..."
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  -t $(IMAGE_NAME):$(IMAGE_TAG) \
	  -t $(IMAGE_NAME):latest \
	  .
	@echo "==> Image ready: $(IMAGE_NAME):$(IMAGE_TAG)"

# ── docker-push ───────────────────────────────────────────────────────────────

## Push the image previously built by docker-build to the registry.
docker-push:
	@echo "==> Pushing $(IMAGE_NAME):$(IMAGE_TAG)..."
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(IMAGE_NAME):latest

# ── docker-buildx ─────────────────────────────────────────────────────────────

## Build and push a multi-arch image (linux/amd64 + linux/arm64) via buildx.
## Requires a buildx builder: docker buildx create --use
docker-buildx:
	@echo "==> Building and pushing multi-arch image for $(PLATFORMS)..."
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --build-arg VERSION=$(VERSION) \
	  -t $(IMAGE_NAME):$(IMAGE_TAG) \
	  -t $(IMAGE_NAME):latest \
	  --push \
	  .
	@echo "==> Pushed: $(IMAGE_NAME):$(IMAGE_TAG)"

# ── clean ─────────────────────────────────────────────────────────────────────

## Remove compiled binaries, embed copy, frontend dist, and generated swagger docs.
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(DIST_DIR)
	rm -rf $(FRONTEND_DIST)
	rm -rf $(DOCS_DIR)
	rm -rf $(EMBED_DIR)
	@echo "==> Clean."

# ── help ──────────────────────────────────────────────────────────────────────

## Show available targets and their descriptions.
help:
	@echo ""
	@echo "memex $(VERSION)"
	@echo ""
	@awk '/^##/{desc=substr($$0,4); next} /^[a-z][a-z_-]+:/{print "  " $$1 "\t" desc; desc=""}' \
	  $(MAKEFILE_LIST) | column -t -s $$'\t'
	@echo ""
