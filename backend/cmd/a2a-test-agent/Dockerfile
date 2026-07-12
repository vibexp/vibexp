# Minimal image for the e2e-only A2A dummy agent (backend/cmd/a2a-test-agent).
#
# Built from the backend module so it reuses the exact a2a-go SDK version the
# backend depends on — no extra dependency, no `replace` directive. Used only by
# docker-compose.e2e.yml to give the Playwright suite a real A2A agent to
# register and chat with; it is never part of the shipped combined image.
#
# Build context is the repo root (so it can see backend/).
FROM golang:1.25.12-alpine AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /a2a-test-agent ./cmd/a2a-test-agent

FROM alpine:3.20
COPY --from=build /a2a-test-agent /usr/local/bin/a2a-test-agent
EXPOSE 9001
# --host must be the name other containers reach this service by (set in
# docker-compose.e2e.yml), since it is baked into the advertised agent-card URL.
ENTRYPOINT ["a2a-test-agent"]
