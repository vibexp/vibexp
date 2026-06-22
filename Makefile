.PHONY: backend-test backend-test-coverage backend-test-integration backend-mock-generate backend-test-clean backend-format backend-vet backend-build backend-download-deps backend-validate-openapi backend-bundle-openapi backend-generate-openapi-server backend-wire-gen backend-wire-check backend-lint-openapi backend-lint backend-vulncheck backend-security backend-check backend-check-migrations backend-run backend-run-dev frontend-install frontend-lint frontend-type-check frontend-test frontend-build frontend-run-dev

# ============================================
# Toolchain Pinning
# ============================================

# Pin the Go toolchain so local development uses the exact same version as CI
# (.github/workflows/ci-backend.yml). GOTOOLCHAIN forces this version even when
# the system Go is newer — the go.mod `toolchain` directive only upgrades, never
# downgrades — which keeps govulncheck, staticcheck and the analyzers
# reproducible everywhere. Go downloads the toolchain on demand if missing.
# Keep GO_VERSION in sync with the go-version pins in the CI workflow.
GO_VERSION := 1.25.11
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

# Regenerate the oapi-codegen strict-server bindings from the bundle, one
# self-contained package per spec-first domain (Notifications #1713, Types
# #1846). Output is committed; CI fails the PR when it is stale relative to the
# spec.
backend-generate-openapi-server: backend-bundle-openapi
	@echo "🧬 Generating OpenAPI strict-server code (Notifications)..."
	@cd backend && go tool oapi-codegen -config oapi-codegen.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Types)..."
	@cd backend && mkdir -p internal/server/gen/types && go tool oapi-codegen -config oapi-codegen-types.yaml dist/openapi.bundled.yaml

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
	if [ ! -f backend/.env ]; then \
			echo "📋 backend/.env not found — copying from .env.example (dev defaults)"; \
			cp backend/.env.example backend/.env; \
		fi && \
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
	if [ ! -f backend/.env ]; then \
			echo "📋 backend/.env not found — copying from .env.example (dev defaults)"; \
			cp backend/.env.example backend/.env; \
		fi && \
	echo "🐘 Starting PostgreSQL..." && \
	cd backend && $$COMPOSE_CMD up postgres -d && \
	echo "⏳ Waiting for PostgreSQL to be ready..." && \
	sleep 3 && \
	echo "🔥 Starting backend with hot reload..." && \
	echo "📝 Watching for .go file changes..." && \
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

# Build the frontend for production
frontend-build:
	cd frontend && npm run build

# Run the frontend application in development mode
frontend-run-dev:
	@if [ ! -f frontend/.env ]; then \
		echo "📋 frontend/.env not found — copying from .env.example (dev defaults)"; \
		cp frontend/.env.example frontend/.env; \
	fi
	@if [ ! -d frontend/node_modules ]; then \
		echo "📦 Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	@echo "🚀 Starting frontend dev server (http://localhost:5173)..."
	@echo "Press Ctrl+C to stop"
	@cd frontend && npm run dev
