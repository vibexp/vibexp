import crypto from 'node:crypto'

import { test, expect } from '@playwright/test'

/**
 * Feature Test: MCP OAuth consent login-gate
 *
 * Closes the e2e gap on the public `/oauth/consent` route (issue #66), the
 * security-sensitive MCP consent flow reworked in #54/#55/#62. The embedded
 * Authorization Server never authenticates anyone itself: /oauth2/authorize
 * stashes a user-LESS login session and redirects to this SPA gate. When the
 * visitor is signed out, the page must bounce them to the app login with a
 * return_to back to the same consent URL.
 *
 * We drive a real authorization request (dynamic client registration + PKCE) to
 * mint a genuine login session, then assert that hitting the consent page while
 * unauthenticated redirects to /login?return_to=<consent url>. A full
 * approve/deny is exercised by backend tests; this proves the SPA gate.
 */

function base64url(buf: Buffer): string {
  return buf
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

test.describe('MCP OAuth consent', () => {
  test('signed-out consent visit is redirected to login with return_to', async ({
    page,
    request,
    baseURL,
  }) => {
    const origin = new URL(baseURL ?? 'http://localhost:8080').origin

    // 1) Register a public OAuth client (dynamic client registration) that may
    //    request the "mcp" scope.
    const regResp = await request.post(`${origin}/oauth2/register`, {
      data: {
        redirect_uris: [`${origin}/callback`],
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

    // 2) Start an authorization request (PKCE S256). The AS validates it, stashes
    //    a user-less login session, and 302-redirects to the SPA consent gate.
    const verifier = base64url(crypto.randomBytes(32))
    const challenge = base64url(
      crypto.createHash('sha256').update(verifier).digest()
    )
    const authorizeUrl =
      `${origin}/oauth2/authorize?response_type=code` +
      `&client_id=${encodeURIComponent(clientId)}` +
      `&redirect_uri=${encodeURIComponent(`${origin}/callback`)}` +
      `&code_challenge=${challenge}&code_challenge_method=S256` +
      `&scope=mcp&state=e2estate12345678` +
      `&resource=${encodeURIComponent(`${origin}/mcp/v1/common`)}`

    const authResp = await request.get(authorizeUrl, { maxRedirects: 0 })
    expect(authResp.status()).toBe(302)
    const location = authResp.headers()['location']
    expect(location).toContain('/oauth/consent?login=')
    const login = new URL(location).searchParams.get('login')
    expect(login).toBeTruthy()

    // 3) Visit the consent gate while signed out → it must redirect to the app
    //    login page with a return_to back to this exact consent URL.
    await page.goto(`/oauth/consent?login=${login}`)
    await page.waitForURL(/\/login\?return_to=/, { timeout: 10000 })

    const returnTo = new URL(page.url()).searchParams.get('return_to')
    expect(returnTo).toContain('/oauth/consent')
    expect(returnTo).toContain(login!)
  })
})
