.PHONY: backend-test backend-test-coverage backend-test-integration backend-mock-generate backend-test-clean backend-format backend-vet backend-build backend-download-deps backend-validate-openapi backend-bundle-openapi backend-generate-openapi-bundle backend-openapi-bundle-check backend-generate-openapi-server backend-wire-gen backend-wire-check backend-generate-config-schema backend-config-schema-check backend-lint-openapi backend-lint backend-vulncheck backend-security backend-check backend-check-migrations backend-run backend-run-dev frontend-install frontend-lint frontend-type-check frontend-test frontend-test-coverage frontend-build frontend-run-dev build-combined e2e-up e2e-down e2e-browsers e2e-test e2e

# ============================================
# Toolchain Pinning
# ============================================

# Pin the Go toolchain so local development uses the exact same version as CI
# (.github/workflows/ci-backend.yml). GOTOOLCHAIN forces this version even when
# the system Go is newer — the go.mod `toolchain` directive only upgrades, never
# downgrades — which keeps govulncheck, staticcheck and the analyzers
# reproducible everywhere. Go downloads the toolchain on demand if missing.
# Keep GO_VERSION in sync with the go-version pins in the CI workflow.
GO_VERSION := 1.25.12
export GOTOOLCHAIN := go$(GO_VERSION)

# ============================================
# Container Runtime Detection Helper
# ============================================

# Helper script to detect and return the appropriate compose command
DETECT_COMPOSE = \
	if command -v podman-compose > /dev/null 2>&1 && podman info > /dev/null 2>&1; then \
		echo "podman-compose"; \
	elif command -v docker-compose > /dev/null 2>&1 && docker info > /dev/null 2>&1; then \
		echo "docker-compose"; \
	elif command -v podman-compose > /dev/null 2>&1; then \
		echo "podman-compose"; \
	else \
		echo "❌ Error: Neither docker-compose nor podman-compose is available." >&2; \
		exit 1; \
	fi

# ============================================
# Backend API Commands
# ============================================

# Run all tests
backend-test:
	cd backend && go test -race -v ./... -timeout=60s

# Run tests with coverage
backend-test-coverage:
	cd backend && go test -race -coverprofile=coverage.out ./... -timeout=60s
	cd backend && go tool cover -html=coverage.out -o coverage.html

# Run repository integration tests against real Postgres (docker-compose
# locally, service container in CI). Override the target database with
# POSTGRES_TEST_DSN.
backend-test-integration:
	cd backend && go test -race -tags=integration -v ./internal/repositories/postgres/... -timeout=180s

# Generate mocks
backend-mock-generate:
	cd backend && mockery --all

# Clean test artifacts
backend-test-clean:
	cd backend && rm -f coverage.out coverage.html

# Download Go module dependencies
backend-download-deps:
	cd backend && go mod download

# Format Go code using gofmt
backend-format:
	@cd backend && if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "The following files are not properly formatted:"; \
		gofmt -s -l .; \
		exit 1; \
	fi

# Run go vet for static analysis
backend-vet:
	cd backend && go vet ./...

# Build the application
backend-build:
	cd backend && go build -ldflags "-X github.com/vibexp/vibexp/cmd.buildSHA=$(shell git rev-parse --short HEAD)" -v ./...

# Validate OpenAPI specification
backend-validate-openapi:
	@echo "🔍 Validating OpenAPI specification..."
	@cd backend && npx @apidevtools/swagger-cli validate openapi.yaml

# Bundle the multi-file spec (root index + paths/ + schemas/) into the single
# artifact consumed by linting, docs, and client generation (#1697).
backend-bundle-openapi:
	@echo "📦 Bundling OpenAPI specification..."
	@cd backend && npx @redocly/cli bundle openapi.yaml -o dist/openapi.bundled.yaml

# Generate the committed, embedded OpenAPI bundle served at /openapi.yaml and
# /openapi.json (#139). The bundled artifacts live inside the embedding package
# (go:embed cannot reach ../../openapi.yaml). The redocly version is pinned so
# the committed bytes are reproducible in CI and pre-commit, where
# backend-openapi-bundle-check regenerates and diffs them (same convention as
# backend-config-schema-check). redocly omits the trailing newline on JSON
# output, so we append one — keeping the committed file identical to what a fresh
# regenerate produces and leaving the end-of-file-fixer hook nothing to change.
REDOCLY_VERSION := 2.5.0
OPENAPI_BUNDLE_DIR := internal/server/openapispec
backend-generate-openapi-bundle:
	@echo "📦 Generating embedded OpenAPI bundle (openapispec)..."
	@cd backend && npx --yes @redocly/cli@$(REDOCLY_VERSION) bundle openapi.yaml -o $(OPENAPI_BUNDLE_DIR)/openapi.bundled.yaml
	@cd backend && npx --yes @redocly/cli@$(REDOCLY_VERSION) bundle openapi.yaml -o $(OPENAPI_BUNDLE_DIR)/openapi.bundled.json
	@cd backend && printf '\n' >> $(OPENAPI_BUNDLE_DIR)/openapi.bundled.json

# Regenerate the embedded bundle and fail if it drifts from the committed files
# — the served spec must be byte-for-byte a fresh bundle of the split source.
# Same gate pattern as backend-config-schema-check / backend-wire-check.
backend-openapi-bundle-check: backend-generate-openapi-bundle
	@cd backend && git diff --exit-code $(OPENAPI_BUNDLE_DIR)/openapi.bundled.yaml $(OPENAPI_BUNDLE_DIR)/openapi.bundled.json \
		|| { echo "❌ embedded OpenAPI bundle is out of sync — run 'make backend-generate-openapi-bundle' and commit the result"; exit 1; }

# Regenerate the oapi-codegen strict-server bindings from the bundle, one
# self-contained package per spec-first domain (Notifications #1713, Types
# #1846). Output is committed; CI fails the PR when it is stale relative to the
# spec.
backend-generate-openapi-server: backend-bundle-openapi
	@echo "🧬 Generating OpenAPI strict-server code (Notifications)..."
	@cd backend && go tool oapi-codegen -config oapi-codegen.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Types)..."
	@cd backend && mkdir -p internal/server/gen/types && go tool oapi-codegen -config oapi-codegen-types.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Team Roles)..."
	@cd backend && mkdir -p internal/server/gen/teamroles && go tool oapi-codegen -config oapi-codegen-teamroles.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Comments)..."
	@cd backend && mkdir -p internal/server/gen/comments && go tool oapi-codegen -config oapi-codegen-comments.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Admin)..."
	@cd backend && mkdir -p internal/server/gen/admin && go tool oapi-codegen -config oapi-codegen-admin.yaml dist/openapi.bundled.yaml

# Regenerate the Wire dependency-injection bindings
# (internal/container/wire_gen.go) from the provider set. Wire is pinned via the
# go.mod `tool` directive so the run is reproducible in CI without `-mod=mod`
# (#1783). Output is committed.
backend-wire-gen:
	@echo "🔌 Regenerating Wire DI code (internal/container)..."
	@cd backend && go tool wire ./internal/container

# Regenerate wire_gen.go, then fail if it differs from the committed file —
# catches DI drift (a hand-edited wire_gen.go, or a changed provider signature
# that was never regenerated). Idempotent on a clean tree (#1783).
backend-wire-check: backend-wire-gen
	@cd backend && git diff --exit-code internal/container/wire_gen.go \
		|| { echo "❌ wire_gen.go is out of sync — run 'make backend-wire-gen' and commit the result"; exit 1; }

# Regenerate the config JSON schema (backend/config.schema.json) from the nested
# config.Config struct. The schema gives editors (VS Code / JetBrains via the
# YAML language server) validation + autocomplete for config.yaml and
# config.example.yaml. Output is committed; CI fails the PR when it is stale
# relative to the struct (backend-config-schema-check).
backend-generate-config-schema:
	@echo "🧬 Generating config JSON schema (backend/config.schema.json)..."
	@cd backend && go run ./cmd/gen-config-schema

# Regenerate config.schema.json, then fail if it differs from the committed file
# — catches a changed Config struct that was never regenerated. Idempotent on a
# clean tree.
backend-config-schema-check: backend-generate-config-schema
	@cd backend && git diff --exit-code config.schema.json \
		|| { echo "❌ config.schema.json is out of sync — run 'make backend-generate-config-schema' and commit the result"; exit 1; }

# Lint the bundle with vacuum using the shared Spectral-format ruleset.
# vacuum (Go) has no nimma, so duplicated-entry-in-enum is active again.
backend-lint-openapi: backend-bundle-openapi
	@echo "🔍 Linting bundled OpenAPI specification with vacuum..."
	@cd backend && npx @quobix/vacuum lint --ruleset ../.github/linters/spectral.yml --details --fail-severity error dist/openapi.bundled.yaml

# Run comprehensive linting (includes gofmt, govet, staticcheck, and more)
backend-lint:
	@echo "🔎 Running golangci-lint (includes gofmt, govet, and more)..."
	cd backend && golangci-lint run --config ../.github/linters/golangci.yml ./...

# Run vulnerability check
backend-vulncheck:
	@echo "🔍 Running vulnerability check..."
	cd backend && govulncheck ./...

# Run security scan
# G706 (log injection) is excluded: the backend logs via log/slog with constant
# message strings and user-derived data only ever in structured attributes, which
# slog's JSON/Text handlers escape and quote — so they are not injectable. G706's
# taint analysis flags these structured-logging calls as false positives.
backend-security:
	@echo "🔒 Running security scan..."
	cd backend && gosec -fmt=text -exclude-generated -exclude-dir=.worktree -exclude=G706 ./...

# Run all code quality and security checks
backend-check: backend-lint backend-vulncheck backend-security
	@echo "\n✅ All code quality and security checks passed!"

# Validate database migrations (local mode - no merge check)
backend-check-migrations:
	@cd backend && bash ../.github/scripts/check-duplicate-migrations.sh migrations false

# Run the backend application locally
backend-run:
	@echo "🔧 Detecting container runtime..."
	@COMPOSE_CMD=$$($(DETECT_COMPOSE)) && \
	echo "✓ Using: $$COMPOSE_CMD" && \
	echo "🔧 Loading environment variables from .env..." && \
	bash scripts/sync-env.sh backend && \
	echo "🐘 Starting PostgreSQL..." && \
	cd backend && $$COMPOSE_CMD up postgres -d && \
	echo "⏳ Waiting for PostgreSQL to be ready..." && \
	sleep 3 && \
	echo "🚀 Starting backend application..." && \
	export $$(grep -v '^#' .env | xargs) && go run . & \
	APP_PID=$$!; \
	echo "⏳ Waiting for application to start..."; \
	sleep 5; \
	echo "🏥 Checking health endpoint..."; \
	if curl -f -s http://localhost:8080/health > /dev/null 2>&1; then \
		echo "✅ Backend is running successfully!"; \
		echo "📊 Health check: http://localhost:8080/health"; \
		echo "🔍 PID: $$APP_PID"; \
		echo ""; \
		echo "Press Ctrl+C to stop the application"; \
		wait $$APP_PID; \
	else \
		echo "❌ Health check failed - backend may not be running correctly"; \
		kill $$APP_PID 2>/dev/null || true; \
		exit 1; \
	fi

# Run the backend application with hot reload (development mode)
backend-run-dev:
	@echo "🔧 Checking for Air installation..."
	@which air > /dev/null 2>&1 || (echo "❌ Air is not installed. Install with: go install github.com/air-verse/air@latest" && exit 1)
	@echo "🔧 Detecting container runtime..."
	@COMPOSE_CMD=$$($(DETECT_COMPOSE)) && \
	echo "✓ Using: $$COMPOSE_CMD" && \
	echo "🔧 Loading environment variables from .env..." && \
	bash scripts/sync-env.sh backend && \
	echo "🐘 Starting PostgreSQL, Mailpit and the local embeddings service..." && \
	cd backend && $$COMPOSE_CMD up postgres mailpit embeddings -d && \
	echo "⏳ Waiting for PostgreSQL to be ready..." && \
	sleep 3 && \
	echo "🔥 Starting backend with hot reload..." && \
	echo "📝 Watching for .go file changes..." && \
	echo "📬 Mailpit (local email inbox) UI: http://localhost:8025" && \
	echo "🧬 Embeddings service (mxbai-embed-large-v1) health: http://localhost:$${TEI_PORT:-8000}/health (first start downloads ~670MB)" && \
	echo "Press Ctrl+C to stop" && \
	echo "" && \
	export $$(grep -v '^#' .env | xargs) && air -c .air.toml

# ============================================
# Frontend Commands
# ============================================

# Install frontend dependencies
frontend-install:
	cd frontend && npm install

# Lint the frontend
frontend-lint:
	cd frontend && npm run lint

# Type-check the frontend
frontend-type-check:
	cd frontend && npm run type-check

# Run the frontend test suite
frontend-test:
	cd frontend && npm run test

# Run the frontend test suite with coverage (writes frontend/coverage/lcov.info,
# consumed by the SonarCloud scan in ci-sonar.yml)
frontend-test-coverage:
	cd frontend && npm run test:coverage

# Build the frontend for production
frontend-build:
	cd frontend && npm run build

# Run the frontend application in development mode
frontend-run-dev:
	@bash scripts/sync-env.sh frontend
	@if [ ! -d frontend/node_modules ]; then \
		echo "📦 Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	@echo "🚀 Starting frontend dev server (http://localhost:5173)..."
	@echo "Press Ctrl+C to stop"
	@cd frontend && npm run dev

# ============================================
# Combined image / binary (issue #61)
# ============================================

# Build the single combined binary: build the frontend SPA, embed it into the Go
# backend (the embedfrontend build tag → spa_embed.go), and produce one binary
# that serves the SPA AND the API from one port. This mirrors what the release
# image does (backend/Dockerfile). Local development does NOT need this — run
# `make backend-run-dev` and `make frontend-run-dev` as two independent
# processes; the backend builds and runs fine with no embedded frontend.
build-combined: frontend-build
	@echo "📦 Embedding frontend build into the backend (internal/server/dist)..."
	rm -rf backend/internal/server/dist
	cp -r frontend/dist backend/internal/server/dist
	@echo "🔨 Building combined binary (backend/bin/vibexp)..."
	cd backend && go build -tags embedfrontend -ldflags "-X github.com/vibexp/vibexp/cmd.buildSHA=$(shell git rev-parse --short HEAD)" -o bin/vibexp .
	@echo "✅ Combined binary built: backend/bin/vibexp"

# ============================================
# End-to-end tests (Playwright) — issue #66
# ============================================
#
# Production-like e2e: docker-compose.e2e.yml builds the combined image from
# source (backend serves the SPA + API on one port) alongside Postgres and a
# fake-gcs-server, so the Playwright suite runs against the artifact we ship.
# CI (.github/workflows/ci-e2e.yml, workflow_dispatch) runs the SAME `make e2e`,
# so local and CI stay identical.
E2E_COMPOSE := docker compose -f docker-compose.e2e.yml
E2E_BASE_URL ?= http://localhost:8080
# URL the BACKEND (app container) uses to reach the dummy A2A agent — the
# compose service name, resolvable inside the vibexp-e2e network. The real-agent
# journey spec registers this URL. Local dev runs default to loopback:9001.
E2E_A2A_AGENT_URL ?= http://a2a-test-agent:9001

# Build + start the e2e stack and block until every service is healthy.
e2e-up:
	@echo "🐳 Building and starting the e2e stack (postgres + fake-gcs + combined app)..."
	$(E2E_COMPOSE) up -d --build --wait --wait-timeout 600

# Tear the stack down and wipe its volumes/network.
e2e-down:
	$(E2E_COMPOSE) down -v --remove-orphans

# Install the Playwright browser(s) the suite needs (chromium only).
e2e-browsers:
	cd frontend && npx playwright install --with-deps chromium

# Run the Playwright suite against an already-running stack.
e2e-test:
	cd frontend && CI=true PLAYWRIGHT_BASE_URL=$(E2E_BASE_URL) E2E_A2A_AGENT_URL=$(E2E_A2A_AGENT_URL) npm run test:e2e

# One-shot: install browsers, bring the stack up, run the suite, always tear the
# stack down, and propagate the suite's exit code. This is what CI runs.
e2e: e2e-browsers e2e-up
	@cd frontend && CI=true PLAYWRIGHT_BASE_URL=$(E2E_BASE_URL) E2E_A2A_AGENT_URL=$(E2E_A2A_AGENT_URL) npm run test:e2e; \
	status=$$?; \
	echo "🧹 Tearing down the e2e stack..."; \
	$(E2E_COMPOSE) down -v --remove-orphans; \
	exit $$status
