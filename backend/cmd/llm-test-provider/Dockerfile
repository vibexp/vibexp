# Minimal image for the e2e-only OpenAI-compatible model stub
# (backend/cmd/llm-test-provider).
#
# Built from the backend module so it needs no module of its own. Used only by
# docker-compose.e2e.yml to give the Playwright suite a deterministic model
# provider for AI Summary; it is never part of the shipped combined image.
#
# Build context is the repo root (so it can see backend/).
FROM golang:1.26.8-alpine AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /llm-test-provider ./cmd/llm-test-provider

FROM alpine:3.20
RUN addgroup -S stub && adduser -S -G stub stub
COPY --from=build /llm-test-provider /usr/local/bin/llm-test-provider
# Drop root: the stub only binds :9002 (unprivileged) and writes nothing.
USER stub
EXPOSE 9002
ENTRYPOINT ["llm-test-provider"]
