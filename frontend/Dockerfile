# syntax=docker/dockerfile:1
#
# Frontend image — standalone Vite/React SPA served by nginx.
#
#   docker build -f frontend/Dockerfile ./frontend
#
# The frontend consumes @vibexp/api-client and @vibexp/design-system from the
# public npm registry, so the build is fully standalone — no monorepo/workspace
# context and no auth token.
#
# API access is same-origin: the SPA is built with a RELATIVE base URL
# (/api/v1) and nginx reverse-proxies /api/ to the backend (BACKEND_ORIGIN,
# default http://backend:8080). This keeps the published image
# deployment-agnostic — the backend location is chosen at deploy time, never
# baked at build time.

# ---- Build stage ----
FROM node:22-alpine AS builder

WORKDIR /app

# Install dependencies first so this layer is cached until the lockfile changes.
COPY package.json package-lock.json .npmrc ./
RUN npm ci

# VITE_* values are inlined into the bundle at build time, so keep them
# deployment-neutral. The API base URL is relative (served behind the nginx
# proxy below); optional integrations default to disabled.
ARG VITE_API_BASE_URL=/api/v1
ARG VITE_GTM_ENABLED=false
ARG VITE_GTM_ID=
ARG VITE_GA4_MEASUREMENT_ID=
ARG VITE_SENTRY_DSN=
ARG VITE_RELEASE_SHA=dev
ARG VITE_RELEASE_DATE=unknown
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_GTM_ENABLED=$VITE_GTM_ENABLED \
    VITE_GTM_ID=$VITE_GTM_ID \
    VITE_GA4_MEASUREMENT_ID=$VITE_GA4_MEASUREMENT_ID \
    VITE_SENTRY_DSN=$VITE_SENTRY_DSN \
    VITE_RELEASE_SHA=$VITE_RELEASE_SHA \
    VITE_RELEASE_DATE=$VITE_RELEASE_DATE

COPY . .
RUN npm run build

# ---- Runtime stage ----
FROM nginx:1.27-alpine

# Reverse-proxy target for /api/. Override at deploy time (compose points it at
# the backend service). The nginx image renders /etc/nginx/templates/*.template
# with envsubst on startup, substituting only declared env vars — so nginx's own
# runtime variables ($uri, $host, …) are preserved.
ENV BACKEND_ORIGIN=http://backend:8080

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

EXPOSE 80
