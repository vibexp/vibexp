import {
  type Browser,
  type BrowserContext,
  expect,
  type Locator,
  type Page,
  test,
} from '@playwright/test'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'

/**
 * Instance site info, end to end (issue #1364, epic #1352).
 *
 * As the instance admin: store company branding, then see it on the sign-in
 * page (signed out), in the app shell (sidebar brand, user menu, tab title)
 * and beside the permanent VibeXP mark in the sidebar footer. Clear the legal
 * URLs and see the legal line leave the sign-in page.
 *
 * ## Why this is an e2e
 *
 * Every slice is unit-tested with the site info context injected. Three
 * things only a browser against the real stack can show:
 *
 * - **The whole chain.** The values travel database → `/config.js`
 *   (`window.__VIBEXP_SITE_INFO__`) → `SiteInfoProvider` → page. Nothing else
 *   proves the provider is mounted in `App.tsx` or that `/config.js` seeds it:
 *   `useSiteInfo` answers "nothing configured" outside a provider by design,
 *   so a missing mount leaves the unit suite green and the app unbranded.
 * - **The logo actually loads.** A broken image falls back to the VibeXP mark,
 *   so "an image is in the lockup" is asserted through `naturalWidth`.
 * - **The footer tooltip is anchored on the mark.** jsdom has no layout
 *   engine, so a trigger that lost its box (#891: a `display: contents`
 *   wrapper, bubble at the viewport origin) is invisible to it. The expected
 *   position is derived from the mark's own box, never from a constant.
 *
 * ## The singleton
 *
 * Site info is one row shared by every spec in the run, and there is no admin
 * page for it yet (#1358), so the journey reads and writes it through the
 * admin API. It records what it found and restores exactly that afterwards (a
 * stored row or "nothing stored"), also when a test fails. No other spec
 * asserts on the brand text, so the change is safe to make mid-run.
 */

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

const ADMIN_PATH = '/api/v1/admin/settings/site-info'
const PRODUCT_URL = 'https://vibexp.io'

/**
 * How far the tooltip may sit from the mark it labels, edge to edge. The
 * bubble's own offset is 4px; the rest is slack for sub-pixel layout. Against
 * the #891 regression the gap is hundreds of pixels, so this discriminates.
 */
const ANCHOR_TOLERANCE = 12

interface SiteInfoValues {
  company_name: string
  company_email: string
  website: string
  support_url: string
  terms_url: string
  privacy_url: string
  logo_url: string
  logo_dark_url: string
}

interface AdminSiteInfo {
  values: SiteInfoValues
  version: number | null
}

interface Box {
  x: number
  y: number
  width: number
  height: number
}

async function readSiteInfo(page: Page): Promise<AdminSiteInfo> {
  const response = await page.request.get(ADMIN_PATH)
  expect(response.status(), 'read site info').toBe(200)
  return (await response.json()) as AdminSiteInfo
}

async function putSiteInfo(page: Page, values: SiteInfoValues): Promise<void> {
  const response = await page.request.put(ADMIN_PATH, { data: values })
  expect(
    response.ok(),
    `store site info: ${String(response.status())} ${await response.text()}`
  ).toBe(true)
}

/** Put the site info back to what `original` read. */
async function restore(page: Page, original: AdminSiteInfo): Promise<void> {
  const response =
    original.version === null
      ? await page.request.delete(ADMIN_PATH)
      : await page.request.put(ADMIN_PATH, { data: original.values })
  expect(response.ok(), `restore failed: ${String(response.status())}`).toBe(
    true
  )
}

/**
 * An entering tooltip slides and zooms, so a box read mid-flight is a moving
 * position. Zero the durations rather than sleeping; `0s`, not `none`, because
 * Radix unmounts on `animationend`.
 */
async function neutraliseAnimations(page: Page): Promise<void> {
  await page.addStyleTag({
    content:
      '*, *::before, *::after { animation-duration: 0s !important; animation-delay: 0s !important; transition-duration: 0s !important; }',
  })
}

async function boxOf(locator: Locator, what: string): Promise<Box> {
  const box = await locator.boundingBox()
  expect(box, `${what} has no layout box`).not.toBeNull()
  return box as Box
}

/** Shortest edge-to-edge distance between two boxes (0 when they touch). */
function gapBetween(a: Box, b: Box): number {
  const dx = Math.max(a.x - (b.x + b.width), b.x - (a.x + a.width), 0)
  const dy = Math.max(a.y - (b.y + b.height), b.y - (a.y + a.height), 0)
  return Math.hypot(dx, dy)
}

/** A fresh, signed-out browser on the sign-in page. */
async function openSignIn(
  browser: Browser
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext()
  const page = await context.newPage()
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: /^Sign in to / })).toBeVisible(
    { timeout: UI_TIMEOUT }
  )
  return { context, page }
}

/** Asserts an `<img>` has actually decoded, not merely that it is in the DOM. */
async function expectImageLoaded(image: Locator, what: string): Promise<void> {
  await expect
    .poll(
      async () =>
        image.evaluate(el => (el as HTMLImageElement).naturalWidth > 0),
      { message: `${what} never loaded`, timeout: UI_TIMEOUT }
    )
    .toBe(true)
}

test.describe.serial('Instance site info branding', () => {
  let adminContext: BrowserContext | undefined
  let adminPage: Page
  let original: AdminSiteInfo | undefined
  // Scoped per ATTEMPT, not per file: `describe.serial` re-runs `beforeAll` on
  // a retry, and a file-scoped name would match a previous attempt's state.
  let branded: SiteInfoValues

  test.beforeAll(async ({ browser, baseURL }, testInfo) => {
    const tag = `${Date.now().toString(36)}${String(testInfo.retry)}`
    branded = {
      company_name: `Acme E2E ${tag}`,
      company_email: 'hello@acme-e2e.test',
      website: 'https://www.acme-e2e.test/about',
      support_url: 'https://acme-e2e.test/help',
      terms_url: 'https://acme-e2e.test/terms',
      privacy_url: 'https://acme-e2e.test/privacy',
      // The instance's own bundled logo: a real, reachable image with no
      // dependency on an external host.
      logo_url: new URL('/logo_rounded.png', baseURL).toString(),
      logo_dark_url: '',
    }

    adminContext = await browser.newContext()
    adminPage = await adminContext.newPage()
    await devLogin(adminPage, ADMIN_EMAIL, ADMIN_NAME)
    original = await readSiteInfo(adminPage)
    await putSiteInfo(adminPage, branded)
  })

  // `original` is only set once the admin page exists and the read succeeded,
  // so a `beforeAll` that failed partway restores nothing and the cleanup does
  // not mask the original error.
  test.afterAll(async () => {
    if (original) await restore(adminPage, original)
    await adminContext?.close()
  })

  test('the sign-in page carries the company branding and credits VibeXP', async ({
    browser,
  }) => {
    const { context, page } = await openSignIn(browser)
    try {
      await expect(
        page.getByRole('heading', {
          name: `Sign in to ${branded.company_name}`,
        })
      ).toBeVisible()

      // The company logo leads the card, and it really loaded.
      const logo = page.getByTestId('sign-in-card-brand').getByRole('img', {
        name: branded.company_name,
      })
      await expect(logo).toBeVisible()
      await expect(logo).toHaveAttribute('src', branded.logo_url)
      await expectImageLoaded(logo, 'the company logo on the sign-in card')

      // The legal line names both documents.
      await expect(
        page.getByText(/By continuing, you agree to our/)
      ).toBeVisible()
      await expect(page.getByRole('link', { name: 'Terms' })).toHaveAttribute(
        'href',
        branded.terms_url
      )
      await expect(
        page.getByRole('link', { name: 'Privacy Policy' })
      ).toHaveAttribute('href', branded.privacy_url)

      // Copyright, site and support in the footer row that is on screen.
      const footer = page
        .getByTestId('sign-in-footer')
        .filter({ visible: true })
      await expect(footer).toHaveCount(1)
      await expect(footer).toContainText(
        `© ${String(new Date().getFullYear())} ${branded.company_name}`
      )
      await expect(
        footer.getByRole('link', { name: 'www.acme-e2e.test' })
      ).toHaveAttribute('href', branded.website)
      await expect(
        footer.getByRole('link', { name: 'support' })
      ).toHaveAttribute('href', branded.support_url)

      // Exactly one "Powered by VibeXP" is on screen, and it goes to vibexp.io.
      const credit = page
        .getByRole('link', { name: /Powered by VibeXP/ })
        .filter({ visible: true })
      await expect(credit).toHaveCount(1)
      await expect(credit).toHaveAttribute('href', PRODUCT_URL)
      await expect(credit).toHaveAttribute('target', '_blank')
    } finally {
      await context.close()
    }
  })

  test('the app shell shows the company in the sidebar, the user menu and the tab title', async () => {
    // A fresh load, so `/config.js` hands the page the stored values.
    await adminPage.goto('/')
    const sidebar = adminPage.getByTestId('app-sidebar')
    await expect(sidebar).toBeVisible({ timeout: UI_TIMEOUT })

    await expect(adminPage).toHaveTitle(`${branded.company_name} · VibeXP`)

    // Sidebar brand: the logo, loaded, and the company name; no VibeXP tagline.
    const brand = sidebar.locator('a[href="/"]').first()
    const logo = brand.getByRole('img', { name: branded.company_name })
    await expect(logo).toHaveAttribute('src', branded.logo_url)
    await expectImageLoaded(logo, 'the company logo in the sidebar')
    await expect(brand).toContainText(branded.company_name)
    await expect(brand).not.toContainText("Your team's shared brain")

    // User menu: the three links, each opening in a new tab.
    await adminPage.getByTestId('user-menu').click()
    const menu = adminPage.getByRole('menu')
    await expect(menu).toBeVisible()
    for (const [label, href] of [
      ['Support', branded.support_url],
      ['Terms', branded.terms_url],
      ['Privacy', branded.privacy_url],
    ] as const) {
      const item = menu.getByRole('menuitem', { name: label })
      await expect(item).toHaveAttribute('href', href)
      await expect(item).toHaveAttribute('target', '_blank')
    }
    await adminPage.keyboard.press('Escape')
    await expect(menu).toBeHidden()
  })

  test('the VibeXP footer mark stays in the sidebar and its tooltip is anchored on it', async () => {
    await adminPage.goto('/')
    await neutraliseAnimations(adminPage)
    const sidebar = adminPage.getByTestId('app-sidebar')
    await expect(sidebar).toBeVisible({ timeout: UI_TIMEOUT })
    const mark = sidebar.getByTestId('sidebar-product-mark').getByRole('link')

    // Expanded: the mark is on screen with its label, crediting VibeXP under
    // the company's branding. There is no tooltip here on purpose: the label
    // already says it.
    await expect(sidebar).toHaveAttribute('data-state', 'expanded')
    await expect(mark).toBeVisible()
    await expect(mark).toHaveAttribute('href', PRODUCT_URL)
    await expect(mark).toHaveAttribute('target', '_blank')
    await expect(mark.getByText('Powered by VibeXP')).toBeVisible()

    // Rail: the mark is still there, alone, and the label moves to a tooltip.
    await adminPage.getByTestId('nav-toggle').click()
    await expect(sidebar).toHaveAttribute('data-state', 'collapsed')
    await expect(mark).toBeVisible()
    await expect(mark).toHaveAttribute('href', PRODUCT_URL)
    await expect(mark.getByText('Powered by VibeXP')).toBeHidden()

    await mark.hover()
    // The wrapper floating-ui positions, rather than `role="tooltip"`: that
    // role sits on a 1px visually-hidden copy inside the bubble.
    const bubble = adminPage
      .locator('[data-radix-popper-content-wrapper]')
      .filter({ hasText: 'Powered by VibeXP' })
    await expect(bubble).toBeVisible()

    // The mark must have a box of its own. A `display: contents` wrapper has
    // none, which is exactly what sends the bubble to the viewport origin.
    const markBox = await boxOf(mark, 'the VibeXP footer mark')
    expect(markBox.width).toBeGreaterThan(0)
    expect(markBox.height).toBeGreaterThan(0)
    // The mark sits at the foot of the sidebar, far from the viewport origin.
    // That distance is what lets the check below tell an anchored bubble from
    // one parked at the origin; were the mark up there too, both would pass.
    expect(Math.hypot(markBox.x, markBox.y)).toBeGreaterThan(
      ANCHOR_TOLERANCE * 10
    )

    // The expected position is derived from the mark's own box. Polled
    // because the popper places itself in a post-layout effect.
    await expect
      .poll(
        async () => {
          const box = await bubble.boundingBox()
          return box ? gapBetween(box, markBox) : Number.POSITIVE_INFINITY
        },
        {
          message:
            'the tooltip never settled beside the VibeXP footer mark — it is anchored on the viewport origin, not on the mark',
        }
      )
      .toBeLessThanOrEqual(ANCHOR_TOLERANCE)

    // Leave the sidebar as it was found for the next test.
    await adminPage.mouse.move(600, 300)
    await adminPage.getByTestId('nav-toggle').click()
    await expect(sidebar).toHaveAttribute('data-state', 'expanded')
  })

  test('clearing the terms and privacy URLs removes the legal line from sign-in', async ({
    browser,
  }) => {
    await putSiteInfo(adminPage, { ...branded, terms_url: '', privacy_url: '' })

    const { context, page } = await openSignIn(browser)
    try {
      // The control: the page is still the company's, so what follows is
      // about the two URLs and not about a page that failed to brand at all.
      await expect(
        page.getByRole('heading', {
          name: `Sign in to ${branded.company_name}`,
        })
      ).toBeVisible()

      await expect(page.getByText(/By continuing, you agree/)).toHaveCount(0)
      await expect(page.getByRole('link', { name: 'Terms' })).toHaveCount(0)
      await expect(
        page.getByRole('link', { name: 'Privacy Policy' })
      ).toHaveCount(0)
    } finally {
      await context.close()
    }
  })
})
