.PHONY: backend-test backend-test-coverage backend-test-coverage-integration backend-test-unit-coverage backend-test-integration-coverage backend-check-integration-shard backend-test-integration backend-mock-generate backend-test-clean backend-format backend-vet backend-build backend-download-deps backend-validate-openapi backend-bundle-openapi backend-generate-openapi-bundle backend-openapi-bundle-check backend-generate-openapi-server backend-openapi-server-check backend-mock-check backend-wire-gen backend-wire-check backend-generate-config-schema backend-config-schema-check backend-lint-openapi backend-lint backend-vulncheck backend-security backend-check backend-check-migrations backend-run backend-run-dev frontend-install frontend-ci-install frontend-lint frontend-type-check frontend-test frontend-test-coverage frontend-audit frontend-build frontend-run-dev build-combined e2e-up e2e-down frontend-deps e2e-browsers e2e-test e2e

# ============================================
# Toolchain Pinning
# ============================================

# Pin the Go toolchain so local development uses the exact same version as CI
# (.github/workflows/ci.yml). GOTOOLCHAIN forces this version even when
# the system Go is newer — the go.mod `toolchain` directive only upgrades, never
# downgrades — which keeps govulncheck, staticcheck and the analyzers
# reproducible everywhere. Go downloads the toolchain on demand if missing.
# Keep GO_VERSION in sync with the go-version pins in the CI workflow.
GO_VERSION := 1.25.13
export GOTOOLCHAIN := go$(GO_VERSION)

# Pin the code generators whose output is committed and drift-gated. A
# regenerate-and-diff gate is only meaningful against a pinned generator: an
# unpinned bump silently rewrites every generated file and turns a version skew
# between a contributor's machine and CI into a red build on untouched code
# (the same reason golangci-lint/gosec/govulncheck are pinned in ci.yml).
# `wire` and `oapi-codegen` need no variable here — they are pinned by the
# `tool` directive in backend/go.mod and invoked with `go tool`.
REDOCLY_VERSION := 2.5.0
MOCKERY_VERSION := v2.53.6

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

# Single-execution full suite (unit + integration-tagged) with coverage. CI no
# longer uses this — it shards into backend-test-unit-coverage and
# backend-test-integration-coverage (#638) — but it is kept as the documented
# LOCAL one-shot, and as the reference the two halves are checked against.
# Needs a reachable Postgres (docker-compose locally, service container in CI);
# override the target database with POSTGRES_TEST_DSN. The longer timeout
# covers the integration harness's one-time migration bootstrap.
backend-test-coverage-integration:
	cd backend && go test -race -tags=integration -coverprofile=coverage.out ./... -timeout=300s

# The two halves CI runs as separate jobs (#638). Together they cover exactly
# what backend-test-coverage-integration covers in one process; that target is
# kept because it remains the documented local one-shot.
#
# Untagged: the whole module WITHOUT `-tags=integration`, so every
# `//go:build integration` file is excluded at build time and no database is
# needed. NOTE the trap recorded in the team notes — a `*_integration_test.go`
# NAME does not imply the tag; all 20 such files under internal/server/ are
# untagged handler tests and run here.
backend-test-unit-coverage:
	cd backend && go test -race -coverprofile=coverage-unit.out ./... -timeout=120s

# The packages that carry `//go:build integration` files — the SINGLE source of
# truth for both the target below and the guard after it, so the two can never
# disagree. Note each package owns its own test database (see the respective
# main_integration_test.go); do NOT set POSTGRES_TEST_DSN across both, that
# points them at one database and reintroduces the collision they avoid.
INTEGRATION_TAGGED_PKGS := internal/repositories/postgres internal/scheduler internal/services/projectmigration
INTEGRATION_TEST_PATTERNS := $(addprefix ./,$(addsuffix /...,$(INTEGRATION_TAGGED_PKGS)))

# Tagged half. Needs a reachable Postgres (docker-compose locally, service
# container in CI). The longer timeout covers the integration harness's one-time
# migration bootstrap.
backend-test-integration-coverage:
	cd backend && go test -race -tags=integration -coverprofile=coverage-integration.out \
		$(INTEGRATION_TEST_PATTERNS) -timeout=300s

# The sharding is only exhaustive while INTEGRATION_TAGGED_PKGS matches reality.
# Add a `//go:build integration` file to a THIRD package and its tagged tests
# would silently stop running in CI — green, and never executed. This gate makes
# that a build failure instead.
backend-check-integration-shard:
	@cd backend && actual=$$(grep -rl '//go:build integration' --include='*.go' . \
		| sed 's|^\./||; s|/[^/]*$$||' | sort -u); \
	expected=$$(printf '%s\n' $(INTEGRATION_TAGGED_PKGS) | sort -u); \
	if [ "$$actual" != "$$expected" ]; then \
		echo "❌ integration-tagged packages have changed — the CI shard would miss tests."; \
		echo "   expected (INTEGRATION_TAGGED_PKGS):"; \
		printf '     %s\n' $$expected; \
		echo "   actually tagged in the tree:"; \
		printf '     %s\n' $$actual; \
		echo "   Fix: update INTEGRATION_TAGGED_PKGS in the Makefile (the test target derives from it)."; \
		exit 1; \
	fi; \
	echo "✅ integration-tagged packages match the CI shard."

# Run repository integration tests against real Postgres (docker-compose
# locally, service container in CI). Override the target database with
# POSTGRES_TEST_DSN.
backend-test-integration:
	cd backend && go test -race -tags=integration -v ./internal/repositories/postgres/... -timeout=180s

# Regenerate the mockery mocks (~124 files under **/mocks/) from .mockery.yaml.
# Invoked via `go run ...@$(MOCKERY_VERSION)` rather than a `mockery` binary off
# PATH so the committed bytes are reproducible for backend-mock-check. Output is
# committed. Runs WITHOUT --all (#678): the per-package `interfaces:` lists in
# backend/.mockery.yaml are authoritative — mockery emits exactly those
# interfaces, so adding a mockable interface means adding its entry there first.
backend-mock-generate:
	@echo "🎭 Regenerating mocks (mockery $(MOCKERY_VERSION))..."
	@cd backend && go run github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

# Regenerate the mocks, then fail if they differ from the committed files —
# catches a hand-edited mock and an interface change that was never regenerated.
# Unlike the other gates this checks `git status` rather than `git diff`, because
# a new interface produces a brand-new (untracked) mock file that `git diff`
# would not see. Idempotent on a clean tree (#635).
backend-mock-check: backend-mock-generate
	@cd backend && if [ -n "$$(git status --porcelain -- ':(glob)**/mocks/*.go')" ]; then \
		git status --short -- ':(glob)**/mocks/*.go'; \
		echo "❌ mocks are out of sync — run 'make backend-mock-generate' and commit the result"; \
		exit 1; \
	fi

# Clean test artifacts
backend-test-clean:
	cd backend && rm -f coverage.out coverage.html coverage-unit.out coverage-integration.out

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
	@cd backend && npx --yes @redocly/cli@$(REDOCLY_VERSION) bundle openapi.yaml -o dist/openapi.bundled.yaml

# Generate the committed, embedded OpenAPI bundle served at /openapi.yaml and
# /openapi.json (#139). The bundled artifacts live inside the embedding package
# (go:embed cannot reach ../../openapi.yaml). The redocly version is pinned so
# the committed bytes are reproducible in CI and pre-commit, where
# backend-openapi-bundle-check regenerates and diffs them (same convention as
# backend-config-schema-check). redocly omits the trailing newline on JSON
# output, so we append one — keeping the committed file identical to what a fresh
# regenerate produces and leaving the end-of-file-fixer hook nothing to change.
# (REDOCLY_VERSION is pinned in the Toolchain Pinning section at the top.)
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
# #1846). Output is committed; backend-openapi-server-check fails the PR when it
# is stale relative to the spec.
backend-generate-openapi-server: backend-bundle-openapi
	@echo "🧬 Generating OpenAPI strict-server code (Notifications)..."
	@cd backend && go tool oapi-codegen -config oapi-codegen.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Types)..."
	@cd backend && mkdir -p internal/server/gen/types && go tool oapi-codegen -config oapi-codegen-types.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Team Roles)..."
	@cd backend && mkdir -p internal/server/gen/teamroles && go tool oapi-codegen -config oapi-codegen-teamroles.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Comments)..."
	@cd backend && mkdir -p internal/server/gen/comments && go tool oapi-codegen -config oapi-codegen-comments.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Relations)..."
	@cd backend && mkdir -p internal/server/gen/relations && go tool oapi-codegen -config oapi-codegen-relations.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Team Settings)..."
	@cd backend && mkdir -p internal/server/gen/teamsettings && go tool oapi-codegen -config oapi-codegen-teamsettings.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Freshness)..."
	@cd backend && mkdir -p internal/server/gen/freshness && go tool oapi-codegen -config oapi-codegen-freshness.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Metadata)..."
	@cd backend && mkdir -p internal/server/gen/metadata && go tool oapi-codegen -config oapi-codegen-metadata.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Admin)..."
	@cd backend && mkdir -p internal/server/gen/admin && go tool oapi-codegen -config oapi-codegen-admin.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Embedding Providers)..."
	@cd backend && mkdir -p internal/server/gen/embeddingproviders && go tool oapi-codegen -config oapi-codegen-embedding-providers.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Memories)..."
	@cd backend && mkdir -p internal/server/gen/memories && go tool oapi-codegen -config oapi-codegen-memories.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Artifacts)..."
	@cd backend && mkdir -p internal/server/gen/artifacts && go tool oapi-codegen -config oapi-codegen-artifacts.yaml dist/openapi.bundled.yaml
	@echo "🧬 Generating OpenAPI strict-server code (Blueprints)..."
	@cd backend && mkdir -p internal/server/gen/blueprints && go tool oapi-codegen -config oapi-codegen-blueprints.yaml dist/openapi.bundled.yaml

# Regenerate the strict-server bindings, then fail if they differ from the
# committed files — catches a hand-edited *.gen.go and a spec change that was
# never regenerated. Checks `git status` (not `git diff`) so a newly generated
# package shows up as untracked rather than passing silently. Idempotent on a
# clean tree (#635).
backend-openapi-server-check: backend-generate-openapi-server
	@cd backend && if [ -n "$$(git status --porcelain -- internal/server/gen)" ]; then \
		git status --short -- internal/server/gen; \
		echo "❌ OpenAPI strict-server bindings are out of sync — run 'make backend-generate-openapi-server' and commit the result"; \
		exit 1; \
	fi

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

# Install frontend dependencies (local development — honours package.json, so
# adding a dependency works). CI uses frontend-ci-install instead.
frontend-install:
	cd frontend && npm install

# Deterministic install from the lockfile, for CI (#638). Kept separate from
# frontend-install so local `make frontend-install` keeps `npm install`
# semantics, and separate from frontend-deps, whose node_modules guard exists
# for the e2e targets and would silently skip a reinstall.
frontend-ci-install:
	cd frontend && npm ci

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

# Audit production frontend dependencies (npm audit --omit=dev, gated at
# moderate+ by frontend/scripts/audit-deps.js). Run by CI and by the
# `frontend dependency audit` pre-commit hook.
frontend-audit:
	cd frontend && npm run audit:deps

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

# Absolute compose path: every e2e target must work regardless of the shell's
# cwd, so a recipe that cd's somewhere first can never break the teardown (#598).
E2E_COMPOSE := docker compose -f $(CURDIR)/docker-compose.e2e.yml
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

# Install the Playwright browser(s) the suite needs (chromium only). Depends on
# frontend-deps so `npx` resolves the pinned local playwright rather than
# downloading its own, and so the two never run concurrently under `make -j`.
#
# `--with-deps` shells out to `sudo apt-get install` for Chromium's shared
# libraries whenever the caller is not root — every run, even when the packages
# are already there — so a plain `make e2e` stopped for a password prompt in the
# middle of an otherwise unattended run (#640). Probe with `install-deps
# --dry-run` (non-zero exit = something is missing, no system changes) and only
# escalate when there is actually something to install: a warm machine takes the
# browser-only path with no sudo, a fresh CI runner takes the original one.
e2e-browsers: frontend-deps
	cd frontend && if npx playwright install-deps --dry-run chromium >/dev/null 2>&1; then \
		npx playwright install chromium; \
	else \
		npx playwright install --with-deps chromium; \
	fi

# The Playwright invocation, defined once and shared by `e2e-test` and `e2e` so
# the two can never drift. It cd's, so callers that do anything afterwards must
# run it in a subshell — see `e2e` below.
E2E_PLAYWRIGHT = cd frontend && CI=true PLAYWRIGHT_BASE_URL=$(E2E_BASE_URL) E2E_A2A_AGENT_URL=$(E2E_A2A_AGENT_URL) npm run test:e2e

# A fresh clone or git worktree has no frontend/node_modules, which surfaces from
# the e2e targets as a bare `sh: 1: playwright: not found` (exit 127) — after the
# stack has already been built and started. `npm ci` (not `install`) because the
# suite must run against the locked dependency set. CI installs deps in its own
# step, so this guard is a no-op there.
frontend-deps:
	@if [ ! -d frontend/node_modules ]; then \
		echo "📦 Installing frontend dependencies..."; \
		cd frontend && npm ci; \
	fi

# Run the Playwright suite against an already-running stack.
e2e-test: frontend-deps
	$(E2E_PLAYWRIGHT)

# One-shot: install browsers, bring the stack up, run the suite, always tear the
# stack down — including on Ctrl-C — and propagate the suite's exit code. This is
# what CI runs.
#
# The suite runs in a SUBSHELL so its `cd frontend` cannot leak into the steps
# after it: unparenthesised, the cd was still in effect at teardown, which then
# failed on the compose path and leaked the whole running stack plus its Postgres
# volume into the next run (#598). The subshell also keeps Playwright's own exit
# code, which a `$(MAKE) e2e-test` sub-make would flatten to make's generic 2.
e2e: frontend-deps e2e-browsers e2e-up
	@trap 'echo "🧹 Tearing down the e2e stack (interrupted)..."; $(MAKE) --no-print-directory e2e-down; exit 130' INT TERM; \
	( $(E2E_PLAYWRIGHT) ); \
	status=$$?; \
	echo "🧹 Tearing down the e2e stack..."; \
	$(MAKE) --no-print-directory e2e-down; \
	exit $$status
