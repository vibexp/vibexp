# PoC: compact MCP endpoints

Throwaway helpers for the `poc/mcp-compact-catalog` branch. Not for merge.

Endpoints served by a backend built from this branch (same bearer token for all):

- `/mcp/v1/common` — every tool registered (unchanged).
- `/mcp/v1/compact` — `vibexp_io_search_tools` + `vibexp_io_call_tool`.
- `/mcp/v1/compact-split` — `vibexp_io_search_tools` + one executor per risk
  class (`call_read_tool`, `call_write_tool`, `call_delete_tool`).

## Run locally

```bash
docker run -d --name vibexp-poc-pg -e POSTGRES_DB=vibexp_io -e POSTGRES_USER=vibexp_app \
  -e POSTGRES_PASSWORD=local_password -p 127.0.0.1:15432:5432 pgvector/pgvector:pg17
cd backend && cp config.example.yaml config.yaml && cp .env.example .env
# in config.yaml: server.port "18080", database.port "15432",
# frontend.base_url "http://localhost:18080", auth.oauth_as.access_token_ttl "12h"
set -a && . ./.env && set +a && go run .
```

A localhost `frontend.base_url` is what enables dev-login and the embedded
authorization server, so no identity provider or key is needed.

## Get a token

`python3 scripts/poc-mcp-compact/mint-token.py` runs dynamic client
registration, PKCE, dev-login and consent against `POC_ORIGIN` (default
`http://localhost:18080`) and writes the access token to `/tmp/vibexp-poc-token`.
It holds no secret. `mcp_client.py` is a minimal JSON-RPC client that reads
that file.
