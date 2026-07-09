# syntax=docker/dockerfile:1
#
# Combined VibeXP image (issue #61): the Go backend embeds the built frontend
# SPA and serves it AND the API from one binary/port — the single-binary,
# one-command self-host model (`docker run ghcr.io/vibexp/vibexp`). No nginx
# proxy, no BACKEND_ORIGIN, no CORS.
#
# The build context is the REPO ROOT (not ./backend), so both frontend/ and
# backend/ are available:
#
#   docker build -f backend/Dockerfile .
#
# Frontend branding/analytics are injected at RUNTIME via /config.js (issue
# #57), so they are NOT build args here; only deployment-neutral, build-time
# values remain (relative API base URL, release stamps, optional Sentry DSN).

# ---- 1) Build the frontend SPA → frontend/dist ----
# Pin to the native builder ($BUILDPLATFORM): the dist is architecture-independent
# (plain JS/CSS), so it is built ONCE on the runner arch and reused for every
# target platform — no QEMU emulation of the npm build.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /app/frontend

# Install deps first so this layer caches until the lockfile changes.
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
RUN npm ci

# Deployment-neutral build-time values. The API is same-origin in the combined
# image, so the base URL is relative; branding/GTM are runtime (config.js).
ARG VITE_API_BASE_URL=/api/v1
ARG VITE_RELEASE_SHA=dev
ARG VITE_RELEASE_DATE=unknown
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_RELEASE_SHA=$VITE_RELEASE_SHA \
    VITE_RELEASE_DATE=$VITE_RELEASE_DATE

COPY frontend/ ./
RUN npm run build

# ---- 2) Build the Go backend, embedding the SPA ----
# Pin to the native builder ($BUILDPLATFORM) and cross-compile per target arch:
# the binary is CGO_ENABLED=0, so GOOS/GOARCH cross-compile for free (no QEMU).
# buildx injects TARGETOS/TARGETARCH for each platform in the manifest list.
FROM --platform=$BUILDPLATFORM golang:1.25.12-alpine AS backend

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go mod and sum files, download dependencies (cached until they change).
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy backend source.
COPY backend/ ./

# Embed the built SPA: the embedfrontend build tag (spa_embed.go) embeds
# ./internal/server/dist, populated here with the frontend build output.
COPY --from=frontend /app/frontend/dist ./internal/server/dist

# Release stamps injected by release.yml (github.sha + build date). The container
# build stage has no .git, so these ldflags are the only source of the SHA/date
# for the binary; when empty (e.g. a plain `docker build`) the -X flags are a
# harmless no-op and the binary falls back to its "dev"/"unknown" sentinels.
ARG RELEASE_SHA=""
ARG RELEASE_DATE=""

# Build with the embedfrontend tag so the SPA is embedded and served.
# GOOS/GOARCH come from buildx's TARGETOS/TARGETARCH so one native build stage
# emits a binary for each target platform (linux/amd64, linux/arm64).
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags "-X github.com/vibexp/vibexp/cmd.buildSHA=${RELEASE_SHA} -X github.com/vibexp/vibexp/cmd.buildDate=${RELEASE_DATE}" \
    -tags embedfrontend -a -installsuffix cgo -o main .

# ---- 3) Minimal runtime ----
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the pre-built binary (with embedded SPA) and migrations.
COPY --from=backend /app/main .
COPY --from=backend /app/migrations ./migrations

# Bake the production-neutral default config (config.docker.yaml) and point the
# binary at it. Every operator knob inside is a ${VAR:-default} reference, so the
# container is configured with environment variables alone — preserving the
# one-command `docker run` experience. Mount your own file over /app/config.yaml
# to replace it: `docker run -v $PWD/config.yaml:/app/config.yaml ...`.
COPY --from=backend /app/config.docker.yaml /app/config.yaml
ENV VIBEXP_CONFIG_FILE=/app/config.yaml

EXPOSE 8080

CMD ["./main"]
