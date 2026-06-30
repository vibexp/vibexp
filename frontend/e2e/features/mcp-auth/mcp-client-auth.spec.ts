import crypto from 'node:crypto'

import { test, expect } from '@playwright/test'

import { devLogin } from '../../fixtures/auth'

/**
 * Feature Test: end-to-end MCP client authorization (issue #34, epic #28).
 *
 * This is the missing real-client leg of the MCP auth suite: it drives the
 * WHOLE OAuth 2.1 flow exactly as an MCP client (Claude Code / Cursor) would —
 * Dynamic Client Registration → /oauth2/authorize (PKCE S256) → app login →
 * consent attach + approve → /oauth2/token → an authenticated call to the MCP
 * resource server — against the production-like combined-image stack
 * (docker-compose.e2e.yml). Unit tests prove each leg in isolation and an
 * in-process Go test proves verifier acceptance; this proves the legs compose
 * over real HTTP through the embedded Authorization Server.
 *
 * The security assertion is precise: the MCP endpoint rejects an anonymous call
 * with 401, and accepts the token minted by the full flow (no 401/403).
 */

function base64url(buf: Buffer): string {
  return buf
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

test.describe('MCP client end-to-end auth', () => {
  test('a token minted via DCR + authorize + consent is accepted by /mcp', async ({
    page,
    baseURL,
  }) => {
    const origin = new URL(baseURL ?? 'http://localhost:8080').origin
    const redirectUri = `${origin}/callback`
    const resource = `${origin}/mcp/v1/common`
    // page.request shares the browser context's cookie jar, so the vx_session
    // set by dev login below is sent automatically on the attach call.
    const api = page.request

    // 1) Dynamic Client Registration (RFC 7591): a public client that may
    //    request the "mcp" scope, like a real MCP client does on first use.
    const regResp = await api.post(`${origin}/oauth2/register`, {
      data: {
        redirect_uris: [redirectUri],
        token_endpoint_auth_method: 'none',
        grant_types: ['authorization_code', 'refresh_token'],
        response_types: ['code'],
        scope: 'mcp',
      },
    })
    expect(regResp.ok()).toBeTruthy()
    const { client_id: clientId } = (await regResp.json()) as {
      client_id: string
    }
    expect(clientId).toBeTruthy()

    // 2) Anonymous MCP call is rejected with 401 + a discovery challenge.
    const anon = await api.post(resource, {
      headers: { Accept: 'application/json, text/event-stream' },
      data: mcpInitialize(),
    })
    expect(anon.status()).toBe(401)
    expect(anon.headers()['www-authenticate']).toContain('resource_metadata')

    // 3) Authorization request (PKCE S256). The AS stashes a user-LESS login
    //    session and 302s to the SPA consent gate.
    const verifier = base64url(crypto.randomBytes(32))
    const challenge = base64url(
      crypto.createHash('sha256').update(verifier).digest()
    )
    const authorizeUrl =
      `${origin}/oauth2/authorize?response_type=code` +
      `&client_id=${encodeURIComponent(clientId)}` +
      `&redirect_uri=${encodeURIComponent(redirectUri)}` +
      `&code_challenge=${challenge}&code_challenge_method=S256` +
      `&scope=mcp&state=e2estate12345678` +
      `&resource=${encodeURIComponent(resource)}`
    const authResp = await api.get(authorizeUrl, { maxRedirects: 0 })
    expect(authResp.status()).toBe(302)
    const login = new URL(authResp.headers()['location']).searchParams.get(
      'login'
    )
    expect(login).toBeTruthy()

    // 4) Sign into the app (dev login → vx_session cookie in the shared jar).
    await devLogin(page)

    // 5) Bind the logged-in user to the login session, then approve consent.
    //    The CSRF token is issued by the consent details endpoint.
    const detailsResp = await api.get(
      `${origin}/api/v1/oauth/consent?login=${login}`
    )
    expect(detailsResp.ok()).toBeTruthy()
    const { csrf } = (await detailsResp.json()) as { csrf: string }
    expect(csrf).toBeTruthy()

    const attachResp = await api.post(`${origin}/api/v1/oauth/consent/attach`, {
      headers: { 'X-CSRF-Token': csrf },
      data: { login },
    })
    expect(attachResp.ok()).toBeTruthy()

    const decisionResp = await api.post(`${origin}/api/v1/oauth/consent`, {
      data: { login, csrf, action: 'approve' },
    })
    expect(decisionResp.ok()).toBeTruthy()
    const { redirect_to: redirectTo } = (await decisionResp.json()) as {
      redirect_to: string
    }
    const code = new URL(redirectTo).searchParams.get('code')
    expect(code).toBeTruthy()

    // 6) Exchange the code for an audience-bound access token.
    const tokenResp = await api.post(`${origin}/oauth2/token`, {
      form: {
        grant_type: 'authorization_code',
        code: code!,
        redirect_uri: redirectUri,
        client_id: clientId,
        code_verifier: verifier,
      },
    })
    expect(tokenResp.ok()).toBeTruthy()
    const { access_token: accessToken } = (await tokenResp.json()) as {
      access_token: string
    }
    expect(accessToken).toBeTruthy()

    // 7) The MCP resource server accepts the freshly-minted token: the call is
    //    no longer 401 (authenticated) nor 403 (scope satisfied).
    const authed = await api.post(resource, {
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: 'application/json, text/event-stream',
      },
      data: mcpInitialize(),
    })
    expect(
      authed.status(),
      'authenticated MCP call must not be 401/403'
    ).not.toBe(401)
    expect(authed.status()).not.toBe(403)
  })
})

// mcpInitialize is a minimal MCP `initialize` JSON-RPC request body; the auth
// layer runs before the protocol layer, so the exact payload only matters for
// not tripping a 4xx that masks the auth result.
function mcpInitialize() {
  return {
    jsonrpc: '2.0',
    id: 1,
    method: 'initialize',
    params: {
      protocolVersion: '2025-11-25',
      capabilities: {},
      clientInfo: { name: 'vibexp-e2e-mcp-client', version: '0.0.0' },
    },
  }
}
