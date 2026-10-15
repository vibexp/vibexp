import { expect, type Page, request, test } from '@playwright/test'

import { devLogin, ONBOARDING_PATH } from '../fixtures/auth'

/**
 * First-run onboarding, end to end: the happy path (#1506, #1508).
 *
 * A brand-new user signs in and the dashboard hands them to the onboarding.
 * They walk all six steps (the concept, the building blocks, the Claude Code
 * plugin, the CLAUDE.md team rules, the CLI) and copy each command. The last
 * step shows the knowledge loop as not started. Then their AI tool reads a
 * memory and writes one back through the REST API with an API key, which is
 * how the CLI and the plugin reach VibeXP. The last step now confirms both,
 * and once the user leaves, the dashboard's setup card is gone for good.
 *
 * ## Why this is an e2e
 *
 * Each piece is unit-tested. Only the real stack shows them working together:
 * the redirect hangs on the `onboarding_completed` flag the backend sends; the
 * loop status hangs on events the backend records for a request it
 * authenticated by API key (the `api` source), never for the web app's own
 * cookie requests; and the copy buttons write the real clipboard.
 */

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

async function expectOnStep(page: Page, step: number): Promise<void> {
  // The flow keeps the step on screen in `?step=` (0-based); the header's
  // "Step N of 6" is the same fact for a screen reader.
  await expect(page).toHaveURL(new RegExp(`[?&]step=${String(step - 1)}$`), {
    timeout: UI_TIMEOUT,
  })
  await expect(page.getByRole('banner')).toContainText(`Step ${String(step)}`)
}

async function clipboard(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText())
}

function loopItem(page: Page, name: RegExp) {
  return page
    .getByRole('list', { name: 'Knowledge loop' })
    .getByRole('listitem')
    .filter({ hasText: name })
}

/**
 * Act as the user's AI tool: with a fresh API key, read one memory and write
 * one, over the REST API and without the browser's session cookie, so the
 * backend records both under the `api` source.
 */
async function runTheLoopAsAnAITool(page: Page): Promise<void> {
  const teams = await page.request.get('/api/v1/teams')
  expect(teams.status(), 'list teams').toBe(200)
  const { teams: teamList } = (await teams.json()) as {
    teams: { id: string }[]
  }
  const teamId = teamList[0]?.id
  expect(teamId, 'the new user has a team').toBeTruthy()

  const projects = await page.request.get(`/api/v1/${teamId}/projects`)
  expect(projects.status(), 'list projects').toBe(200)
  const { projects: projectList } = (await projects.json()) as {
    projects: { id: string }[]
  }
  const projectId = projectList[0]?.id
  expect(projectId, 'the team has a default project').toBeTruthy()

  const created = await page.request.post('/api/v1/api-keys', {
    data: { name: 'Onboarding e2e agent', integration_codes: ['ai_tools'] },
  })
  expect(created.ok(), `create API key: ${created.status()}`).toBe(true)
  const { full_key: apiKey } = (await created.json()) as { full_key: string }

  const agent = await request.newContext({
    baseURL: new URL(page.url()).origin,
    extraHTTPHeaders: { Authorization: `Bearer ${apiKey}` },
  })
  try {
    const write = await agent.post(`/api/v1/${teamId}/memories`, {
      data: {
        project_id: projectId,
        title: 'Onboarding e2e',
        text: 'Every POST to /charges must send an Idempotency-Key header.',
      },
    })
    expect(write.ok(), `write a memory: ${write.status()}`).toBe(true)
    const { id: memoryId } = (await write.json()) as { id: string }

    const read = await agent.get(`/api/v1/${teamId}/memories/${memoryId}`)
    expect(read.status(), 'read the memory back').toBe(200)
  } finally {
    await agent.dispose()
  }
}

test('a new user is onboarded and their AI tool closes the knowledge loop', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await devLogin(
    page,
    `e2e-onboarding-${String(Date.now())}@vibexp.test`,
    'Onboarding Journey',
    { keepOnboarding: true }
  )

  // 1. Welcome: signing in lands on the dashboard, which hands a new user over.
  await expectOnStep(page, 1)
  await expect(
    page.getByRole('heading', {
      name: "Your team's shared brain for AI agents",
    })
  ).toBeVisible()
  await expect(
    page.locator('iframe[src*="youtube-nocookie.com/embed/-9NFGBjoc5g"]')
  ).toBeVisible()
  await page.getByRole('button', { name: 'Get started' }).click()

  // 2. Building blocks.
  await expectOnStep(page, 2)
  const blocks = page.getByRole('tablist', { name: 'Building blocks' })
  await blocks.getByRole('tab', { name: 'Blueprints' }).click()
  await expect(
    page.getByRole('heading', { name: 'Blueprints', level: 3 })
  ).toBeVisible()
  await page.getByRole('button', { name: 'Continue' }).click()

  // 3. Claude Code plugin.
  await expectOnStep(page, 3)
  await page.getByRole('button', { name: 'Copy marketplace command' }).click()
  expect(await clipboard(page)).toBe(
    '/plugin marketplace add vibexp/plugin-marketplace'
  )
  await page.getByRole('button', { name: "I've installed it" }).click()

  // 4. Team rules for ~/.claude/CLAUDE.md.
  await expectOnStep(page, 4)
  await page.getByRole('button', { name: 'Copy rules' }).click()
  const rules = await clipboard(page)
  expect(rules).toContain('/vibexp:prime')
  expect(rules).toContain('/vibexp:wrap')
  await page.getByRole('button', { name: "I've added the rules" }).click()

  // 5. The CLI, set up by Claude for this instance.
  await expectOnStep(page, 5)
  await page.getByRole('button', { name: 'Copy install command' }).click()
  expect(await clipboard(page)).toBe(
    `claude "/vibexp:install ${new URL(page.url()).origin}"`
  )
  await page.getByRole('button', { name: "I've run it" }).click()

  // 6. All set, but no AI tool has read or written yet.
  await expectOnStep(page, 6)
  await expect(
    page.getByRole('heading', { name: 'Your agents now learn with your team' })
  ).toBeVisible()
  await expect(loopItem(page, /read from the shared brain/)).toHaveAttribute(
    'data-state',
    'pending'
  )
  await expect(loopItem(page, /wrote back/)).toHaveAttribute(
    'data-state',
    'pending'
  )

  // The user's AI tool reads and writes; the step confirms both.
  await runTheLoopAsAnAITool(page)
  await page.reload()
  await expectOnStep(page, 6)
  await expect(loopItem(page, /read from the shared brain/)).toHaveAttribute(
    'data-state',
    'done',
    { timeout: UI_TIMEOUT }
  )
  await expect(loopItem(page, /wrote back/)).toHaveAttribute(
    'data-state',
    'done'
  )

  // Leaving stores completion; with the loop running the setup card is gone,
  // and the onboarding stays one click away in the profile menu.
  await page.getByRole('button', { name: 'Go to VibeXP' }).click()
  await page.waitForURL('/', { timeout: UI_TIMEOUT })
  await page.reload()
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: /Welcome back/ })).toBeVisible(
    { timeout: UI_TIMEOUT }
  )
  await expect(page.getByText('Finish setting up VibeXP')).toHaveCount(0)
  await page.getByTestId('user-menu').click()
  await page.getByRole('menuitem', { name: /Getting started/ }).click()
  await expect(page).toHaveURL(new RegExp(`^[^?]*${ONBOARDING_PATH}`))

  const me = await page.request.get('/api/v1/auth/me')
  const user = (await me.json()) as {
    onboarding_completed: boolean
    knowledge_loop: {
      first_read_at: string | null
      first_write_at: string | null
    }
  }
  expect(user.onboarding_completed).toBe(true)
  expect(user.knowledge_loop.first_read_at).not.toBeNull()
  expect(user.knowledge_loop.first_write_at).not.toBeNull()
})
