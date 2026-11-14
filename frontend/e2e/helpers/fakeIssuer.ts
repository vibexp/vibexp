import { type APIRequestContext, expect, type Page } from '@playwright/test'

/**
 * The e2e stack's scriptable OpenID Connect issuer
 * (`backend/cmd/oidc-test-issuer`, #1650): who signs in next, with which
 * claims, and whether the issuer refuses the sign-in or the next refresh.
 *
 * It listens in the app container's network namespace and the app publishes
 * its port, so `http://localhost:<E2E_OIDC_PORT>` is the issuer URL for the
 * browser, for this runner and for the app alike, which checks it against
 * every ID token's `iss`. `make e2e` exports `E2E_OIDC_PORT` (default 9003).
 */
export const ISSUER_URL = `http://localhost:${process.env.E2E_OIDC_PORT ?? '9003'}`

/** The client the issuer accepts (its `--client-id` / `--client-secret`). */
export const ISSUER_CLIENT = {
  id: 'e2e-oidc',
  secret: 'e2e-oidc-client-credential',
}

async function control(
  request: APIRequestContext,
  path: string,
  data?: unknown
): Promise<void> {
  const response = await request.post(`${ISSUER_URL}/_control/${path}`, {
    data,
  })
  expect(response.status(), `issuer control ${path}`).toBe(204)
}

/** Who the next sign-in is, and the extra ID-token claims from now on. */
export async function scriptUser(
  request: APIRequestContext,
  user: { sub: string; email: string; claims?: Record<string, unknown> }
): Promise<void> {
  await control(request, 'user', user)
}

/** Sets one extra claim of every ID token minted from now on, refreshes too. */
export async function scriptClaim(
  request: APIRequestContext,
  name: string,
  value: unknown
): Promise<void> {
  await control(request, 'claims', { name, value })
}

/** Revokes every refresh token: each refresh answers `invalid_grant`. */
export async function refuseRefresh(request: APIRequestContext): Promise<void> {
  await control(request, 'refresh', { error: 'invalid_grant' })
}

/** Refuses every authorization with the RFC 6749 `error` code. */
export async function refuseAuthorization(
  request: APIRequestContext,
  error: string
): Promise<void> {
  await control(request, 'authorize-error', { error })
}

/** Back to the issuer's own user, no claims, no refusal, no tokens. */
export async function resetIssuer(request: APIRequestContext): Promise<void> {
  await control(request, 'reset')
}

/** How many refresh_token grants the issuer has served since it started. */
export async function refreshCalls(
  request: APIRequestContext
): Promise<number> {
  const response = await request.get(`${ISSUER_URL}/_control/stats`)
  expect(response.status()).toBe(200)
  return ((await response.json()) as { refresh_calls: number }).refresh_calls
}

/**
 * Checks the issuer is up and advertises the URL the app is configured with,
 * since an `iss` mismatch would refuse every sign-in for a reason no
 * assertion of the journey names.
 */
export async function expectIssuerReady(
  request: APIRequestContext
): Promise<void> {
  const response = await request.get(
    `${ISSUER_URL}/.well-known/openid-configuration`
  )
  expect(response.status(), `issuer discovery at ${ISSUER_URL}`).toBe(200)
  const doc = (await response.json()) as { issuer: string }
  expect(doc.issuer).toBe(ISSUER_URL)
}

/**
 * Sends the browser through the issuer the way a real one would. The fake
 * issuer answers `/authorize` with its `{code, state}` (or `{error, state}`)
 * as JSON rather than redirecting to the caller's `redirect_uri`, which would
 * be an open redirect; this turns that answer into the 302 to the app's
 * callback, after checking the callback is this app's own.
 */
async function followAuthorizeAsRedirect(page: Page): Promise<void> {
  const callback = new URL('/api/v1/auth/callback', page.url()).toString()
  await page.context().route(
    url => url.origin === ISSUER_URL && url.pathname === '/authorize',
    async route => {
      const asked = new URL(route.request().url()).searchParams.get(
        'redirect_uri'
      )
      expect(asked, 'the app sends its own callback').toBe(callback)
      const response = await route.fetch()
      expect(response.status(), 'issuer /authorize').toBe(200)
      const answer = (await response.json()) as Record<string, string>
      const location = new URL(callback)
      for (const key of ['code', 'state', 'error']) {
        if (answer[key]) location.searchParams.set(key, answer[key])
      }
      await route.fulfill({
        status: 302,
        headers: { location: location.toString() },
      })
    }
  )
}

/**
 * Clicks "Continue with `displayName`" on the sign-in page and follows the
 * sign-in through the issuer back to the app. The caller asserts where it
 * landed: signed in, or on the callback's error page.
 */
export async function signInWithProvider(
  page: Page,
  displayName: string
): Promise<void> {
  await page.goto('/login')
  await followAuthorizeAsRedirect(page)
  await page
    .getByRole('button', { name: `Continue with ${displayName}` })
    .click()
  await page.waitForURL(url => !url.pathname.startsWith('/login'), {
    timeout: 20_000,
  })
}
