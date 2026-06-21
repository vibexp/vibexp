# syntax=docker/dockerfile:1
#
# Frontend image — built from the WORKSPACE ROOT context.
#
#   docker build -f frontend/Dockerfile .
#
# The frontend is an npm-workspace member that depends on the in-repo packages
# @vibexp/api-client and @vibexp/design-system (declared as "*" and resolved via
# the root workspace). A standalone `docker build ./frontend` cannot see those
# sibling packages, so the build context MUST be the repo root. A root
# .dockerignore keeps the context small (it excludes node_modules, the Go
# backend, build outputs, etc.).
#
# The api-client and design-system `dist/` dirs are vendored (committed), so the
# frontend consumes the prebuilt output and NO codegen toolchain runs in the
# image. No private registry / auth token is required at any stage.

# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

# Copy the entire workspace root context (pruned by .dockerignore). npm requires
# every workspace listed in the root package.json to be present on disk to build
# the workspace dependency graph, so we copy the whole tree rather than a subset.
COPY . .

# Install all dependencies (including devDependencies needed for the build).
# Everything resolves from the public npm registry or the local workspace — no
# private registry or auth token required.
RUN npm install

# Build arguments for configuration
ARG VITE_GTM_ID=
ARG VITE_GTM_ENABLED=false
ARG VITE_API_BASE_URL=https://api.vibexp.io/api/v1
ARG VITE_SENTRY_DSN
ARG VITE_GA4_MEASUREMENT_ID
ARG VITE_RELEASE_SHA=dev
ARG VITE_RELEASE_DATE=unknown
ARG VITE_FIREBASE_API_KEY=
ARG VITE_FIREBASE_AUTH_DOMAIN=
ARG VITE_FIREBASE_PROJECT_ID=
ARG VITE_FIREBASE_STORAGE_BUCKET=
ARG VITE_FIREBASE_MESSAGING_SENDER_ID=
ARG VITE_FIREBASE_APP_ID=
ARG VITE_FIREBASE_VAPID_KEY=

# Set environment variables during build
ENV VITE_GTM_ID=$VITE_GTM_ID
ENV VITE_GTM_ENABLED=$VITE_GTM_ENABLED
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN
ENV VITE_GA4_MEASUREMENT_ID=$VITE_GA4_MEASUREMENT_ID
ENV VITE_RELEASE_SHA=$VITE_RELEASE_SHA
ENV VITE_RELEASE_DATE=$VITE_RELEASE_DATE
ENV VITE_FIREBASE_API_KEY=$VITE_FIREBASE_API_KEY
ENV VITE_FIREBASE_AUTH_DOMAIN=$VITE_FIREBASE_AUTH_DOMAIN
ENV VITE_FIREBASE_PROJECT_ID=$VITE_FIREBASE_PROJECT_ID
ENV VITE_FIREBASE_STORAGE_BUCKET=$VITE_FIREBASE_STORAGE_BUCKET
ENV VITE_FIREBASE_MESSAGING_SENDER_ID=$VITE_FIREBASE_MESSAGING_SENDER_ID
ENV VITE_FIREBASE_APP_ID=$VITE_FIREBASE_APP_ID
ENV VITE_FIREBASE_VAPID_KEY=$VITE_FIREBASE_VAPID_KEY

# Build only the frontend workspace. It consumes the prebuilt api-client/dist +
# design-system/dist, so no OpenAPI codegen runs here.
RUN npm run build -w frontend

# Production stage
FROM nginx:1.27-alpine

# Install envsubst (gettext) for the entrypoint script
RUN apk add --no-cache gettext

# Copy the built SPA to the nginx web root
COPY --from=builder /app/frontend/dist /usr/share/nginx/html

# Simple nginx config for an SPA (history-API fallback to index.html)
RUN echo 'server { \
    listen 80; \
    location / { \
        root /usr/share/nginx/html; \
        index index.html index.htm; \
        try_files $uri $uri/ /index.html; \
    } \
}' > /etc/nginx/conf.d/default.conf

# Startup script for runtime environment variable injection
# Note: Runtime injection is limited; build-time environment variables are primary
# This script is kept for backwards compatibility but should not be relied upon
RUN echo '#!/bin/sh' > /docker-entrypoint.sh && \
    echo 'set -e' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Only inject GTM variables if explicitly enabled' >> /docker-entrypoint.sh && \
    echo 'if [ "$VITE_GTM_ENABLED" = "true" ] && [ -n "$VITE_GTM_ID" ]; then' >> /docker-entrypoint.sh && \
    echo '  echo "GTM enabled with ID: $VITE_GTM_ID"' >> /docker-entrypoint.sh && \
    echo 'fi' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Start nginx' >> /docker-entrypoint.sh && \
    echo 'exec nginx -g "daemon off;"' >> /docker-entrypoint.sh && \
    chmod +x /docker-entrypoint.sh

EXPOSE 80

CMD ["/docker-entrypoint.sh"]
