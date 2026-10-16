import {
  type Browser,
  type BrowserContext,
  type Download,
  expect,
  type Locator,
  type Page,
  test,
} from '@playwright/test'
import { readFile } from 'node:fs/promises'

import { ADMIN_EMAIL, ADMIN_NAME } from '../features/admin/admin-emails'
import { devLogin } from '../fixtures/auth'

/**
 * The audit log, end to end (issue #1519, epic #1510).
 *
 * A per-run user signs in with dev login and creates a prompt through the UI.
 * As the instance admin, the Audit tab filtered by that user shows both events
 * with the right operation, actor, channel and outcome; the operation filter
 * narrows the list to the prompt row, the URL carries both filters and a
 * reload reproduces the list; the detail drawer shows the event; and the CSV
 * export of that filter matches the visible rows one-for-one.
 *
 * ## Isolation
 *
 * Every spec in the run writes audit events in parallel, so this one never
 * reads "the newest row": it filters by its own per-run actor
 * (`e2e-audit-<run>@vibexp.test`), whose events nobody else can create.
 *
 * ## Waiting
 *
 * Events are recorded asynchronously (a worker pool persists them after the
 * request returns), so every "the row is there" assertion polls: reload the
 * list until it holds what is expected.
 */

const UI_TIMEOUT = 20_000
/** Long enough for the async recorder under a loaded CI stack. */
const EVENT_TIMEOUT = 30_000

/** The export's columns, in the order #1516 documents them. */
const CSV_COLUMNS = [
  'created_at',
  'operation',
  'outcome',
  'source',
  'actor_type',
  'actor_email',
  'actor_name',
  'actor_user_id',
  'team_name',
  'team_id',
  'project_name',
  'project_id',
  'resource_type',
  'resource_name',
  'resource_id',
  'assistant_name',
  'api_key_name',
  'ip',
  'user_agent',
  'request_id',
  'metadata',
]

/**
 * RFC 4180 rows: quoted fields may hold commas, doubled quotes and line
 * breaks (the metadata column is quoted JSON). Good enough for the export,
 * which this spec controls; not a general CSV parser.
 */
function parseCsv(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let quoted = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') {
        field += '"'
        i++
      } else if (ch === '"') {
        quoted = false
      } else {
        field += ch
      }
    } else if (ch === '"') {
      quoted = true
    } else if (ch === ',') {
      row.push(field)
      field = ''
    } else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && text[i + 1] === '\n') i++
      row.push(field)
      rows.push(row)
      row = []
      field = ''
    } else {
      field += ch
    }
  }
  if (field !== '' || row.length > 0) {
    row.push(field)
    rows.push(row)
  }
  return rows
}

async function signedInPage(
  browser: Browser,
  email: string,
  name: string
): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({
    acceptDownloads: true,
    viewport: { width: 1440, height: 900 },
  })
  const page = await context.newPage()
  await devLogin(page, email, name)
  return { context, page }
}

/** The signed-in user's id, from the session. */
async function userIdOf(page: Page): Promise<string> {
  const response = await page.request.get('/api/v1/auth/me')
  expect(response.status()).toBe(200)
  const me = (await response.json()) as { id?: string }
  expect(me.id, 'auth/me returned no id').toBeTruthy()
  return me.id as string
}

/** The user's personal team, which a new account's prompt lands in. */
async function personalTeamOf(
  page: Page
): Promise<{ id: string; name: string }> {
  const response = await page.request.get('/api/v1/teams')
  expect(response.status()).toBe(200)
  const { teams } = (await response.json()) as {
    teams: { id: string; name: string; is_personal: boolean }[]
  }
  const personal = teams.find(team => team.is_personal)
  expect(personal, 'the new user has no personal team').toBeTruthy()
  return personal as { id: string; name: string }
}

/**
 * A prompt's id and project, read back from the prompt API by the slug its
 * page URL ends in.
 */
async function readPrompt(
  page: Page,
  teamId: string,
  slug: string
): Promise<{ id: string; projectId?: string }> {
  const response = await page.request.get(`/api/v1/${teamId}/prompts/${slug}`)
  expect(response.status()).toBe(200)
  const prompt = (await response.json()) as {
    id: string
    project_id?: string | null
  }
  return { id: prompt.id, projectId: prompt.project_id ?? undefined }
}

/**
 * Create a prompt through the real form, so it is recorded as a web write.
 * Returns the slug the new prompt's page URL ends in.
 */
async function createPromptInUi(page: Page, name: string): Promise<string> {
  await page.goto('/prompts/new')
  const nameField = page.getByTestId('prompt-name-input')
  await expect(nameField).toBeVisible({ timeout: UI_TIMEOUT })
  await nameField.fill(name)
  await page
    .locator('textarea[placeholder*="Write your prompt here"]')
    .fill('Recorded by the audit log journey')
  await page.getByTestId('prompt-save-button').click()
  await expect(page).toHaveURL(/\/prompts\/(?!new$)[^/]+$/, {
    timeout: UI_TIMEOUT,
  })
  return new URL(page.url()).pathname.split('/').pop() as string
}

/** The audit table's body rows (the phone-width cards are hidden here). */
function auditRows(page: Page): Locator {
  return page.getByRole('table').locator('tbody tr')
}

/** Reload the audit list until `check` holds: events land asynchronously. */
async function untilListed(
  page: Page,
  check: () => Promise<void>
): Promise<void> {
  await expect(async () => {
    await page.reload()
    await expect(page.getByRole('table')).toBeVisible({ timeout: 5_000 })
    await check()
  }).toPass({ timeout: EVENT_TIMEOUT, intervals: [1_000, 2_000, 3_000] })
}

/** Click Export CSV and read the download. */
async function exportCsv(page: Page): Promise<{
  filename: string
  rows: string[][]
}> {
  const [download]: [Download, void] = await Promise.all([
    page.waitForEvent('download', { timeout: UI_TIMEOUT }),
    page.getByRole('button', { name: 'Export CSV' }).click(),
  ])
  const path = await download.path()
  const text = await readFile(path, 'utf8')
  return { filename: download.suggestedFilename(), rows: parseCsv(text) }
}

const AUDIT_SETTINGS_PATH = '/api/v1/admin/settings/audit-log'

interface AuditLogSettings {
  source: 'instance' | 'default'
  values: { retention_weeks: number; capture_ip_ua: boolean }
}

async function readAuditSettings(page: Page): Promise<AuditLogSettings> {
  const response = await page.request.get(AUDIT_SETTINGS_PATH)
  expect(response.status()).toBe(200)
  return (await response.json()) as AuditLogSettings
}

/**
 * Put the audit-log settings back to what `original` read: the stored values,
 * or a reset when the built-in defaults applied. `capture_ip_ua` is written
 * back exactly as found; this journey never changes it.
 */
async function restoreAuditSettings(
  page: Page,
  original: AuditLogSettings
): Promise<void> {
  const response =
    original.source === 'default'
      ? await page.request.delete(AUDIT_SETTINGS_PATH)
      : await page.request.put(AUDIT_SETTINGS_PATH, { data: original.values })
  expect(response.ok(), `restore failed: ${String(response.status())}`).toBe(
    true
  )
  const after = await readAuditSettings(page)
  expect(after.source).toBe(original.source)
  expect(after.values).toEqual(original.values)
}

test.describe.serial('Admin → Audit', () => {
  let userContext: BrowserContext | undefined
  let adminContext: BrowserContext | undefined

  test.afterEach(async () => {
    await userContext?.close()
    await adminContext?.close()
  })

  test('a user’s sign-in and prompt show up, filter, open and export', async ({
    browser,
  }, testInfo) => {
    test.setTimeout(180_000)

    const run = `${String(testInfo.workerIndex)}-${Date.now().toString(36)}-${String(testInfo.retry)}`
    const userEmail = `e2e-audit-${run}@vibexp.test`
    const userName = `Audit Journey ${run}`
    const promptName = `Audit journey prompt ${run}`

    // --- The per-run actor signs in and creates a prompt through the UI ---
    const user = await signedInPage(browser, userEmail, userName)
    userContext = user.context
    const userId = await userIdOf(user.page)
    const team = await personalTeamOf(user.page)
    const slug = await createPromptInUi(user.page, promptName)
    const { id: promptId, projectId } = await readPrompt(
      user.page,
      team.id,
      slug
    )

    // --- As the admin, open Admin → Audit from the sidebar ----------------
    const admin = await signedInPage(browser, ADMIN_EMAIL, ADMIN_NAME)
    adminContext = admin.context
    const page = admin.page
    await page.goto('/admin')
    await page
      .getByRole('navigation', { name: 'Admin sections' })
      .getByRole('link', { name: 'Audit', exact: true })
      .click()
    await expect(page).toHaveURL(/\/admin\/audit$/)
    await expect(
      page.getByRole('heading', { name: 'Audit', level: 1 })
    ).toBeVisible()

    // --- Filter by the per-run user through the user picker --------------
    await page.getByRole('combobox', { name: 'Filter by user' }).click()
    await page.getByPlaceholder('Search name or email…').fill(userEmail)
    await page.getByRole('option', { name: new RegExp(userName) }).click()
    await expect(page).toHaveURL(new RegExp(`actor_user_id=${userId}`))
    await expect(
      page.getByRole('combobox', { name: 'Filter by user' })
    ).toHaveText(userName)

    // Both events are there, and nothing but this user's.
    const promptRow = auditRows(page).filter({ hasText: 'Prompt created' })
    const signInRow = auditRows(page).filter({ hasText: 'Signed in' })
    await untilListed(page, async () => {
      await expect(promptRow).toHaveCount(1)
      await expect(signInRow).toHaveCount(1)
    })
    await expect(promptRow).toContainText(userName)
    await expect(promptRow).toContainText(promptName)
    await expect(promptRow).toContainText('Web')
    await expect(promptRow).toContainText('Success')
    // Scope: the user's team (the exact ids are checked on the export).
    await expect(promptRow).toContainText(team.name)
    await expect(signInRow).toContainText(userName)
    await expect(signInRow).toContainText('Success')
    for (const row of await auditRows(page).all()) {
      await expect(row).toContainText(userName)
    }

    // --- Narrow to the prompt with the operation filter ------------------
    await page.getByRole('combobox', { name: 'Filter by operation' }).click()
    await page.getByPlaceholder('Search operations…').fill('Prompt created')
    await page.getByRole('option', { name: 'Prompt created' }).click()
    await page.keyboard.press('Escape')
    await expect(page).toHaveURL(/operation=prompt\.create/)
    await expect(auditRows(page)).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(auditRows(page).first()).toContainText(promptName)

    // The URL carries both filters, so a reload reproduces the list.
    const filtered = new URL(page.url())
    expect(filtered.searchParams.get('actor_user_id')).toBe(userId)
    expect(filtered.searchParams.get('operation')).toBe('prompt.create')
    await page.reload()
    await expect(auditRows(page)).toHaveCount(1, { timeout: UI_TIMEOUT })
    await expect(auditRows(page).first()).toContainText(promptName)
    await expect(
      page.getByRole('combobox', { name: 'Filter by user' })
    ).toHaveText(userName, { timeout: UI_TIMEOUT })
    await expect(
      page.getByRole('combobox', { name: 'Filter by operation' })
    ).toHaveText('Prompt created')

    // --- The detail drawer ------------------------------------------------
    await auditRows(page).first().click()
    const drawer = page.getByRole('dialog')
    await expect(drawer).toBeVisible()
    await expect(
      drawer.getByRole('heading', { name: 'Prompt created' })
    ).toBeVisible()
    await expect(drawer).toContainText('prompt.create')
    await expect(drawer).toContainText(userEmail)
    await expect(drawer).toContainText(userId)
    await expect(drawer).toContainText(promptName)
    await expect(drawer).toContainText(promptId)
    await expect(drawer).toContainText(/Channel\s*Web/)
    await page.keyboard.press('Escape')
    await expect(drawer).toBeHidden()

    // --- The export of that filter matches the visible rows --------------
    const visible = await auditRows(page).count()
    const csv = await exportCsv(page)
    expect(csv.filename).toMatch(/^admin-audit-\d{8}\.csv$/)
    expect(csv.rows[0]).toEqual(CSV_COLUMNS)
    const data = csv.rows.slice(1)
    expect(data).toHaveLength(visible)
    const col = (name: string) => CSV_COLUMNS.indexOf(name)
    for (const record of data) {
      expect(record).toHaveLength(CSV_COLUMNS.length)
      expect(record[col('operation')]).toBe('prompt.create')
      expect(record[col('resource_name')]).toBe(promptName)
      expect(record[col('actor_email')]).toBe(userEmail)
      expect(record[col('source')]).toBe('web')
      expect(record[col('outcome')]).toBe('success')
      expect(record[col('resource_id')]).toBe(promptId)
      expect(record[col('team_id')]).toBe(team.id)
      if (projectId) expect(record[col('project_id')]).toBe(projectId)
    }
  })

  test('a retention change is saved, in the change history and in the audit log', async ({
    browser,
  }) => {
    test.setTimeout(120_000)

    const admin = await signedInPage(browser, ADMIN_EMAIL, ADMIN_NAME)
    adminContext = admin.context
    const page = admin.page
    const adminId = await userIdOf(page)
    const original = await readAuditSettings(page)
    const before = original.values.retention_weeks
    // In 1..52 and never equal to the value found.
    const retention = (before % 52) + 1
    // Over built-in defaults the snapshot's "before" side is null, which the
    // drawer names instead of printing a number.
    const oldShown =
      original.source === 'default' ? 'Built-in defaults' : String(before)

    try {
      // --- Audit tab → "Audit settings" → Settings → Audit log ---------------
      await page.goto('/admin/audit')
      await page.getByRole('link', { name: 'Audit settings' }).click()
      await expect(page).toHaveURL(/\/admin\/settings\/audit-log$/)
      const field = page.getByLabel('Retention', { exact: true })
      await expect(field).toHaveValue(String(before), { timeout: UI_TIMEOUT })

      await field.fill(String(retention))
      await page.getByRole('button', { name: 'Save changes' }).click()
      await expect(page.getByTestId('instance-settings-source')).toHaveText(
        'Customized',
        { timeout: UI_TIMEOUT }
      )

      // The change history shows this save, old → new.
      await expect(
        page
          .getByTestId('instance-audit-log-audit-entry')
          .filter({ hasText: 'Saved' })
          .filter({
            hasText: new RegExp(
              `Retention \\(weeks\\).*\\b${String(before)}\\b.*→ ${String(retention)}\\b`
            ),
          })
          .first()
      ).toBeVisible({ timeout: UI_TIMEOUT })

      // --- The change is itself an audit event ------------------------------
      await page.goto(
        `/admin/audit?actor_user_id=${adminId}&operation=audit.settings.updated`
      )
      const rows = auditRows(page).filter({ hasText: 'Audit settings updated' })
      // Newest first; reopen until this save's event (old → new) is the one.
      await expect(async () => {
        await page.reload()
        await expect(rows.first()).toBeVisible({ timeout: 5_000 })
        await rows.first().click()
        const drawer = page.getByRole('dialog')
        await expect(drawer).toContainText('audit.settings.updated')
        // The row also reads "changed to" for screen readers.
        await expect(
          drawer
            .getByRole('list', { name: 'Changes' })
            .getByRole('listitem')
            .filter({ hasText: 'retention_weeks' })
        ).toHaveText(
          new RegExp(
            `retention_weeks:\\s*${oldShown}\\s*→\\s*changed to\\s*${String(retention)}$`
          )
        )
      }).toPass({ timeout: EVENT_TIMEOUT, intervals: [1_000, 2_000, 3_000] })
    } finally {
      // Pass or fail, put the shared singleton back and verify it.
      await restoreAuditSettings(page, original)
    }
  })
})
