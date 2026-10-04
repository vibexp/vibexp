#!/usr/bin/env python3
"""Mint an MCP access token on the local PoC instance (DCR + PKCE + dev-login + consent).
Writes the token to /tmp/vibexp-poc-token (mode 600) and prints nothing secret."""
import base64, hashlib, os, secrets, sys, urllib.parse, requests

origin = os.environ.get("POC_ORIGIN", "http://localhost:18080")
email = os.environ.get("POC_EMAIL", "poc-harness@example.com")
redirect = origin + "/callback"
resource = origin + "/mcp/v1/common"
s = requests.Session()

def b64(b): return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

client_id = s.post(origin + "/oauth2/register", json={
    "redirect_uris": [redirect], "token_endpoint_auth_method": "none",
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"], "scope": "mcp"}).json()["client_id"]
verifier = b64(secrets.token_bytes(32))
challenge = b64(hashlib.sha256(verifier.encode()).digest())
r = s.get(origin + "/oauth2/authorize", params={
    "response_type": "code", "client_id": client_id, "redirect_uri": redirect,
    "code_challenge": challenge, "code_challenge_method": "S256", "scope": "mcp",
    "state": "pocstate12345678", "resource": resource}, allow_redirects=False)
assert r.status_code == 302, (r.status_code, r.text[:200])
login = urllib.parse.parse_qs(urllib.parse.urlparse(r.headers["location"]).query)["login"][0]
for _ in range(2):  # first login on a fresh DB has no default team yet
    d = s.post(origin + "/api/v1/auth/dev/login", json={"email": email, "name": "PoC Harness"})
    assert d.ok, (d.status_code, d.text[:200])
csrf = s.get(origin + "/api/v1/oauth/consent", params={"login": login}).json()["csrf"]
a = s.post(origin + "/api/v1/oauth/consent/attach", headers={"X-CSRF-Token": csrf}, json={"login": login})
assert a.ok, (a.status_code, a.text[:200])
dec = s.post(origin + "/api/v1/oauth/consent", json={"login": login, "csrf": csrf, "action": "approve"})
assert dec.ok, (dec.status_code, dec.text[:200])
code = urllib.parse.parse_qs(urllib.parse.urlparse(dec.json()["redirect_to"]).query)["code"][0]
tok = s.post(origin + "/oauth2/token", data={
    "grant_type": "authorization_code", "code": code, "redirect_uri": redirect,
    "client_id": client_id, "code_verifier": verifier})
assert tok.ok, (tok.status_code, tok.text[:200])
t = tok.json()
fd = os.open("/tmp/vibexp-poc-token", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, t["access_token"].encode()); os.close(fd)
me = d.json()
print("ok expires_in=%s default_team=%s" % (t.get("expires_in"), me.get("default_team_id") or (me.get("user") or {}).get("default_team_id")))
