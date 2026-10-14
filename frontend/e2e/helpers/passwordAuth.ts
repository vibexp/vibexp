import {
  type Browser,
  type BrowserContext,
  expect,
  type Page,
} from '@playwright/test'

import { execInApp } from './e2eApp'

/**
 * Shared steps of the password journeys (#1444, epic #1426).
 *
 * The e2e stack has no identity provider, so these journeys sign in through
 * the password form, read emailed links from the stack's Mailpit, and change
 * instance-wide settings (password sign-in, registration mode, the instance
 * email provider) that every journey must put back as it found them.
 */

/** A per-run namespace for the addresses a journey creates. */
export const RUN_ID = Date.now().toString(36)

export function runEmail(journey: string, who: string): string {
  return `e2e-pw-${journey}-${who}-${RUN_ID}@vibexp.test`
}

/** A password that passes the policy, unique per person. */
export function passwordFor(who: string): string {
  return `e2e ${who} passphrase ${RUN_ID}`
}

/** Signs in through the sign-in page's password form and waits to leave it. */
export async function passwordSignIn(
  page: Page,
  email: string,
  password: string
): Promise<void> {
  await page.goto('/login')
  const form = page.getByRole('form', { name: 'Password sign-in' })
  await form.getByLabel('Email').fill(email)
  await form.getByLabel('Password', { exact: true }).fill(password)
  await form.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).not.toHaveURL(/\/login/, { timeout: 20_000 })
}

// --- Mailpit -----------------------------------------------------------------

interface MailpitSummary {
  ID: string
  To: { Address: string }[]
  Subject: string
}

function mailpit<T>(path: string): T {
  const body = execInApp([
    'wget',
    '-qO-',
    `http://127.0.0.1:8025/api/v1${path}`,
  ])
  return JSON.parse(body) as T
}

/**
 * The newest link in a mail to `address` whose path matches `pattern`, polled
 * until it arrives. Returned as a path, so it is opened against the suite's
 * base URL whatever host the app put in the mail.
 */
export async function emailedLink(
  address: string,
  pattern: RegExp,
  timeoutMs = 20_000
): Promise<string> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    const { messages } = mailpit<{ messages: MailpitSummary[] }>(
      `/search?query=${encodeURIComponent(`to:${address}`)}`
    )
    for (const message of messages) {
      const full = mailpit<{ Text: string; HTML: string }>(
        `/message/${message.ID}`
      )
      const match = pattern.exec(`${full.Text}\n${full.HTML}`)
      if (match) return match[0]
    }
    if (Date.now() > deadline) {
      throw new Error(`no mail to ${address} with a link matching ${pattern}`)
    }
    await new Promise(resolve => setTimeout(resolve, 500))
  }
}

/** The newest `/account/setup#token=…` or `/account/reset#token=…` link. */
export function emailedAccountLink(
  address: string,
  kind: 'setup' | 'reset'
): Promise<string> {
  return emailedLink(address, new RegExp(`/account/${kind}#token=[\\w-]+`))
}

/** The newest `/invitations/accept/<token>` link. */
export function emailedInvitationLink(address: string): Promise<string> {
  return emailedLink(address, /\/invitations\/accept\/[\w%+-]+/)
}

/** A team the journey owns, created through the API by `page`'s user. */
export async function createTeam(page: Page, name: string): Promise<string> {
  const response = await page.request.post('/api/v1/teams', {
    data: { name, description: 'Password journeys (#1444)' },
  })
  expect(response.ok(), await response.text()).toBeTruthy()
  return ((await response.json()) as { id: string }).id
}

/**
 * An account with a password and nothing else, made the way an administrator
 * makes one: create it, then complete its setup link (taken from the create
 * response, or from Mailpit when the link was emailed).
 */
export async function createPasswordAccount(
  admin: Page,
  browser: Browser,
  email: string,
  password: string
): Promise<void> {
  const response = await admin.request.post('/api/v1/admin/users', {
    data: { email },
  })
  expect(response.ok(), await response.text()).toBeTruthy()
  const { setup } = (await response.json()) as {
    setup: { delivery: string; link_url?: string }
  }
  const link =
    setup.delivery === 'link' && setup.link_url
      ? setup.link_url
      : await emailedAccountLink(email, 'setup')
  const token = link.split('#token=')[1]
  // Completing the link signs its caller in, so it runs in a throwaway context
  // and never touches the admin's session.
  const owner = await browser.newContext()
  try {
    const done = await owner.request.post('/api/v1/auth/password/setup', {
      data: { token, password },
    })
    expect(done.ok(), await done.text()).toBeTruthy()
  } finally {
    await owner.close()
  }
}

// --- Instance-wide settings ----------------------------------------------------

interface AuthPolicy {
  password_login_enabled: boolean
  registration_mode: 'open' | 'invite_only'
}

/** The instance email provider as the stack's journeys configure it. */
export const MAILPIT_PROVIDER = {
  provider_type: 'smtp',
  from_address: 'e2e-password@vibexp.test',
  from_name: 'VibeXP E2E',
  settings: { smtp: { host: '127.0.0.1', port: '1025', username: 'e2e' } },
  secret: 'e2e-mailpit-password',
}

export async function readPolicy(page: Page): Promise<AuthPolicy> {
  const response = await page.request.get('/api/v1/admin/settings/auth/policy')
  expect(response.ok()).toBeTruthy()
  return (await response.json()) as AuthPolicy
}

/** Stores a policy, confirming any lockout risk (journeys set it on purpose). */
export async function setPolicy(page: Page, policy: AuthPolicy): Promise<void> {
  const response = await page.request.put(
    '/api/v1/admin/settings/auth/policy',
    {
      data: { ...policy, confirm_lockout_risk: true },
    }
  )
  expect(response.ok(), await response.text()).toBeTruthy()
  // The server caches the policy for up to two seconds.
  await page.waitForTimeout(2_500)
}

export async function emailConfigured(page: Page): Promise<boolean> {
  const response = await page.request.get('/api/v1/admin/settings/email')
  expect(response.ok()).toBeTruthy()
  return ((await response.json()) as { configured: boolean }).configured
}

export async function useMailpit(page: Page): Promise<void> {
  const response = await page.request.put('/api/v1/admin/settings/email', {
    data: MAILPIT_PROVIDER,
  })
  expect(response.ok(), await response.text()).toBeTruthy()
}

export async function removeEmail(page: Page): Promise<void> {
  const response = await page.request.delete('/api/v1/admin/settings/email')
  expect([204, 409]).toContain(response.status())
}

/**
 * Runs `body` with the instance's password policy and email provider put back
 * afterwards, pass or fail. `page` must be signed in as an instance admin.
 */
export async function withInstanceAuthRestored(
  page: Page,
  body: () => Promise<void>
): Promise<void> {
  const policy = await readPolicy(page)
  const hadEmail = await emailConfigured(page)
  try {
    await body()
  } finally {
    await setPolicy(page, {
      password_login_enabled: policy.password_login_enabled,
      registration_mode: policy.registration_mode,
    })
    if (hadEmail) await useMailpit(page)
    else await removeEmail(page)
  }
}

/** A fresh browser with no session: a different person, or a signed-out one. */
export async function freshPage(
  browser: Browser
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext()
  return { context, page: await context.newPage() }
}

/** The signed-in user of `page`'s browser, or null when signed out. */
export async function whoAmI(page: Page): Promise<string | null> {
  const response = await page.request.get('/api/v1/auth/me')
  if (!response.ok()) return null
  return ((await response.json()) as { email: string }).email
}

/** Whether a password sign-in for the address succeeds, from a fresh browser. */
export async function passwordWorks(
  browser: Browser,
  email: string,
  password: string
): Promise<boolean> {
  const context = await browser.newContext()
  try {
    const response = await context.request.post('/api/v1/auth/password/login', {
      data: { email, password },
    })
    return response.ok()
  } finally {
    await context.close()
  }
}
