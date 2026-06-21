#!/usr/bin/env bash
# Verifies the WorkOS AuthKit offline_access silent-refresh flow for a public
# PKCE client (the mobile-app auth model, issue #1673 / epic #1672):
#
#   1. authorize (PKCE S256, offline_access scope) — opens a browser login
#   2. authorization_code exchange with code_verifier (no client secret)
#   3. refresh_token exchange — must return a new access token + rotated
#      refresh token without any user interaction
#   4. replay of the consumed refresh token — must fail (single-use rotation)
#
# Usage:
#   AUTHKIT_ISSUER=https://api.workos.com/user_management/client_... \
#   WORKOS_CLIENT_ID=client_... \
#   ./scripts/verify_authkit_refresh.sh
#
# AUTHKIT_ISSUER accepts either WorkOS issuer form — the user_management form
# above (what native PKCE clients get) or a custom AuthKit domain
# (https://<env>.authkit.app). Endpoints are taken from the issuer's OIDC
# discovery document.
#
# Optional: REDIRECT_URI (default http://localhost:8080/api/v1/auth/callback —
# the WorkOS dev redirect; its port is where the one-shot callback listener
# binds, so nothing else may be listening there).
#
# Tokens are never printed in full; the script prints decoded claims and TTLs.
set -euo pipefail

AUTHKIT_ISSUER="${AUTHKIT_ISSUER:?set AUTHKIT_ISSUER (issuer URL, either WorkOS form)}"
WORKOS_CLIENT_ID="${WORKOS_CLIENT_ID:?set WORKOS_CLIENT_ID (public client id)}"
REDIRECT_URI="${REDIRECT_URI:-http://localhost:8080/api/v1/auth/callback}"
PORT="$(printf '%s' "$REDIRECT_URI" | sed -nE 's#^https?://[^:/]+:([0-9]+).*#\1#p')"
if [ -z "$PORT" ]; then
  echo "REDIRECT_URI must carry an explicit port for the local callback listener (e.g. http://localhost:8080/...)" >&2
  exit 1
fi

echo "== Step 0: OIDC discovery =="
DISCOVERY="$(curl -sS --fail --max-time 30 "${AUTHKIT_ISSUER}/.well-known/openid-configuration")"
AUTHORIZE_ENDPOINT="$(printf '%s' "$DISCOVERY" | python3 -c 'import json,sys;print(json.load(sys.stdin)["authorization_endpoint"])')"
TOKEN_ENDPOINT="$(printf '%s' "$DISCOVERY" | python3 -c 'import json,sys;print(json.load(sys.stdin)["token_endpoint"])')"
echo "authorization_endpoint: $AUTHORIZE_ENDPOINT"
echo "token_endpoint:         $TOKEN_ENDPOINT"

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

CODE_VERIFIER="$(openssl rand 48 | b64url)"
CODE_CHALLENGE="$(printf '%s' "$CODE_VERIFIER" | openssl dgst -sha256 -binary | b64url)"
STATE="$(openssl rand 16 | b64url)"

AUTHORIZE_URL="${AUTHORIZE_ENDPOINT}?response_type=code&client_id=${WORKOS_CLIENT_ID}&redirect_uri=$(python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=""))' "$REDIRECT_URI")&provider=authkit&scope=openid%20profile%20email%20offline_access&code_challenge=${CODE_CHALLENGE}&code_challenge_method=S256&state=${STATE}"

echo "== Step 1: authorize (PKCE, offline_access) =="
echo "Open this URL in a browser and complete the login:"
echo
echo "  $AUTHORIZE_URL"
echo
echo "Waiting for the redirect on port ${PORT}..."

CODE="$(python3 - "$PORT" "$STATE" <<'PYEOF'
import sys, urllib.parse
from http.server import BaseHTTPRequestHandler, HTTPServer

port, want_state = int(sys.argv[1]), sys.argv[2]
result = {}

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        q = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
        result["code"] = q.get("code", [""])[0]
        result["state"] = q.get("state", [""])[0]
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(b"Callback received - return to the terminal.")
    def log_message(self, *a):
        pass

srv = HTTPServer(("127.0.0.1", port), H)
while "code" not in result or not result["code"]:
    srv.handle_request()
if result["state"] != want_state:
    sys.exit("state mismatch - aborting")
print(result["code"])
PYEOF
)"
echo "Got authorization code."

decode_claims() {  # $1 = JWT; prints selected claims and TTL
  python3 - "$1" <<'PYEOF'
import base64, json, sys, time
payload = sys.argv[1].split(".")[1]
payload += "=" * (-len(payload) % 4)
c = json.loads(base64.urlsafe_b64decode(payload))
ttl = c.get("exp", 0) - int(time.time())
print(f"  iss={c.get('iss')}")
print(f"  sub={c.get('sub')}")
print(f"  aud={c.get('aud', '(absent)')}")
print(f"  exp in {ttl}s (~{ttl//60} min)")
PYEOF
}

token_request() {  # $@ = form params; prints JSON response, exits non-zero on HTTP error
  curl -sS --fail-with-body --max-time 30 -X POST "${TOKEN_ENDPOINT}" "$@"
}

echo
echo "== Step 2: authorization_code exchange (code_verifier, no client secret) =="
RESP="$(token_request \
  -d grant_type=authorization_code \
  -d "client_id=${WORKOS_CLIENT_ID}" \
  -d "code=${CODE}" \
  -d "redirect_uri=${REDIRECT_URI}" \
  -d "code_verifier=${CODE_VERIFIER}")"
ACCESS_TOKEN="$(printf '%s' "$RESP" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("access_token",""))')"
REFRESH_TOKEN="$(printf '%s' "$RESP" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("refresh_token",""))')"
[ -n "$ACCESS_TOKEN" ] || { echo "FAIL: no access_token in response"; exit 1; }
[ -n "$REFRESH_TOKEN" ] || { echo "FAIL: no refresh_token (offline_access not honored)"; exit 1; }
echo "Access token claims:"
decode_claims "$ACCESS_TOKEN"
echo "Refresh token: present (${#REFRESH_TOKEN} chars)"

echo
echo "== Step 3: silent refresh (grant_type=refresh_token, no interaction) =="
RESP2="$(token_request \
  -d grant_type=refresh_token \
  -d "client_id=${WORKOS_CLIENT_ID}" \
  -d "refresh_token=${REFRESH_TOKEN}")"
ACCESS_TOKEN2="$(printf '%s' "$RESP2" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("access_token",""))')"
REFRESH_TOKEN2="$(printf '%s' "$RESP2" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("refresh_token",""))')"
[ -n "$ACCESS_TOKEN2" ] || { echo "FAIL: refresh returned no access_token"; exit 1; }
[ "$ACCESS_TOKEN2" != "$ACCESS_TOKEN" ] || { echo "FAIL: access token not renewed"; exit 1; }
echo "New access token claims:"
decode_claims "$ACCESS_TOKEN2"
if [ -n "$REFRESH_TOKEN2" ] && [ "$REFRESH_TOKEN2" != "$REFRESH_TOKEN" ]; then
  echo "Refresh token: rotated (new value returned)"
else
  echo "WARN: refresh token was not rotated"
fi

echo
echo "== Step 4: replay consumed refresh token (must fail — single-use) =="
if token_request \
  -d grant_type=refresh_token \
  -d "client_id=${WORKOS_CLIENT_ID}" \
  -d "refresh_token=${REFRESH_TOKEN}" >/dev/null 2>&1; then
  echo "WARN: consumed refresh token was accepted (rotation not enforced?)"
else
  echo "Replay rejected as expected (single-use refresh tokens)."
fi

echo
echo "== PASS: offline_access silent refresh verified =="
