# Minimal image for the e2e-only OpenID Connect issuer
# (backend/cmd/oidc-test-issuer).
#
# Built from the backend module so it needs no module of its own. Used only by
# docker-compose.e2e.yml so the Playwright suite can sign in through an
# identity provider without a real one; it is never part of the shipped
# combined image.
#
# Build context is the repo root (so it can see backend/).
FROM golang:1.26.11-alpine AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /oidc-test-issuer ./cmd/oidc-test-issuer

FROM alpine:3.20
RUN addgroup -S stub && adduser -S -G stub stub
COPY --from=build /oidc-test-issuer /usr/local/bin/oidc-test-issuer
# Drop root: the issuer only binds :9003 (unprivileged) and writes nothing.
USER stub
EXPOSE 9003
ENTRYPOINT ["oidc-test-issuer"]
