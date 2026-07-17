import { expect, test, type Browser, type Page } from '@playwright/test'

import { devLogin } from '../../fixtures/auth'

/**
 * E2E happy-path coverage for resource comments (issue #278), exercising the
 * assembled combined stack across two frontend surfaces:
 *  - the sidebar CommentsPanel widget + all-comments popup on a resource detail
 *    page (add, in-place edit, pagination, permission-gated action visibility),
 *  - the homepage "Recent comments" activity card (click-through + cascade).
 *
 * Modelled on the multi-user invitation-accept spec: raw Playwright test with a
 * dedicated browser context per user, team/resource setup via the API (that's
 * scaffolding, not the behaviour under test), and the UI driven for assertions.
 */

const uniqueEmail = (prefix: string) =>
  `${prefix}_${process.env.TEST_WORKER_INDEX ?? '0'}_${Date.now()}@example.com`

// Two users sharing one team: OWNER authors comments and moderates; MEMBER is a
// plain member used for the permission-visibility checks.
let ownerCtx: Awaited<ReturnType<Browser['newContext']>>
let memberCtx: Awaited<ReturnType<Browser['newContext']>>
let ownerPage: Page
let memberPage: Page

let teamId: string
let projectId: string
let artifactId: string
let artifactUrl: string
const stamp = Date.now()
const artifactTitle = `Comments E2E Artifact ${stamp}`
const OWNER_NAME = 'Comments Owner'
const MEMBER_NAME = 'Comments Member'

async function postJson(page: Page, url: string, data: unknown) {
  const res = await page.request.post(url, { data })
  expect(
    res.ok(),
    `POST ${url} failed: ${res.status()} ${await res.text()}`
  ).toBeTruthy()
  return res.json() as Promise<Record<string, unknown>>
}

/**
 * Pin a page's active team from boot. Uses addInitScript (runs before the app
 * reads the team on every navigation) rather than a post-load localStorage
 * write, which races team hydration — under load the artifact then fails to
 * resolve and the currentTeam-gated CommentsPanel never renders.
 */
async function pinTeam(page: Page, id: string) {
  await page.addInitScript(tid => {
    localStorage.setItem('vx_current_team_id', tid)
  }, id)
}

// Generous timeout for widget assertions — the combined e2e stack runs specs in
// parallel, so a comment fetch/mutation can lag well past the 5s expect default.
const WIDGET_TIMEOUT = 20000

/** Wait for the artifact detail page + widget to settle, then return the panel. */
async function commentsPanel(page: Page) {
  // Wait for the artifact's own h1 specifically — the view also renders an h1 in
  // its "Loading…" / "not found" states, so a generic heading gate would pass
  // before the real page (and its currentTeam-gated sidebar) mounts.
  await expect(
    page.getByRole('heading', { level: 1, name: artifactTitle })
  ).toBeVisible({ timeout: WIDGET_TIMEOUT })
  const panel = page.getByTestId('comments-panel')
  await expect(panel).toBeVisible({ timeout: WIDGET_TIMEOUT })
  await expect(page.getByTestId('comments-loading')).toHaveCount(0, {
    timeout: WIDGET_TIMEOUT,
  })
  return panel
}

test.describe.serial('Resource comments happy path', () => {
  test.describe.configure({ timeout: 150_000 })

  test.beforeAll(async ({ browser }) => {
    // OWNER: fresh user, creates the shared team + project + artifact via API.
    ownerCtx = await browser.newContext()
    ownerPage = await ownerCtx.newPage()
    await devLogin(ownerPage, uniqueEmail('e2e_comment_owner'), OWNER_NAME)

    const teamBody = await postJson(ownerPage, '/api/v1/teams', {
      name: `Comments E2E Team ${stamp}`,
      description: 'Created by the resource-comments e2e',
    })
    teamId = ((teamBody.team as { id?: string })?.id ??
      (teamBody.id as string)) as string
    expect(teamId, 'team id missing from create response').toBeTruthy()

    const projectBody = await postJson(
      ownerPage,
      `/api/v1/${teamId}/projects`,
      { name: `E2E Project ${stamp}`, slug: `e2e-project-${stamp}` }
    )
    projectId = projectBody.id as string
    expect(projectId).toBeTruthy()

    const artifactBody = await postJson(
      ownerPage,
      `/api/v1/${teamId}/artifacts`,
      {
        project_id: projectId,
        slug: `e2e-comments-${stamp}`,
        title: artifactTitle,
        content: 'Artifact under comment test.',
      }
    )
    artifactId = artifactBody.id as string
    const artifactSlug = artifactBody.slug as string
    expect(artifactId).toBeTruthy()
    artifactUrl = `/artifacts/${projectId}/${artifactSlug}`

    // MEMBER: fresh user invited into the shared team as a plain member.
    const memberEmail = uniqueEmail('e2e_comment_member')
    // Invitations are nested under /teams/ (unlike projects/artifacts/comments,
    // which are /api/v1/{teamId}/...).
    await postJson(ownerPage, `/api/v1/teams/${teamId}/invitations`, {
      emails: [memberEmail],
      role: 'member',
    })

    memberCtx = await browser.newContext()
    memberPage = await memberCtx.newPage()
    await devLogin(memberPage, memberEmail, MEMBER_NAME)
    const pending = await memberPage.request.get('/api/v1/invitations/pending')
    const pendingBody = (await pending.json()) as {
      invitations?: { token: string }[]
    }
    const token = pendingBody.invitations?.[0]?.token
    expect(token, 'invitee has no pending invitation token').toBeTruthy()
    await memberPage.goto(`/invitations/accept/${encodeURIComponent(token!)}`)
    await memberPage
      .getByRole('button', { name: /^accept(\s+invitation)?$/i })
      .first()
      .click()
    // Confirm real membership before relying on it.
    await expect
      .poll(
        async () => {
          const res = await memberPage.request.get('/api/v1/teams')
          const body = (await res.json()) as { teams?: { id?: string }[] }
          return (body.teams ?? []).some(t => t.id === teamId)
        },
        { timeout: 15000 }
      )
      .toBe(true)

    // Both pages act within the shared team from here on (set before every nav).
    await pinTeam(ownerPage, teamId)
    await pinTeam(memberPage, teamId)
  })

  test.afterAll(async () => {
    await memberCtx?.close()
    await ownerCtx?.close()
  })

  test('owner adds a comment via the sidebar widget', async () => {
    await ownerPage.goto(artifactUrl)
    const panel = await commentsPanel(ownerPage)

    await panel.getByTestId('comment-add-button').click()
    await panel.getByRole('textbox').fill('First comment from the owner')
    await panel.getByRole('button', { name: 'Comment', exact: true }).click()

    const row = panel
      .getByTestId('comment-row')
      .filter({ hasText: 'First comment from the owner' })
    await expect(row).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await expect(row).toContainText(OWNER_NAME, { timeout: WIDGET_TIMEOUT })
  })

  test('owner edits the comment in place and it shows "(edited)"', async () => {
    const panel = await commentsPanel(ownerPage)
    // Only one comment exists at this point; filtering by body text would break
    // once the row swaps into edit mode (the body is replaced by the editor).
    const row = panel.getByTestId('comment-row')
    await expect(row).toHaveCount(1, { timeout: WIDGET_TIMEOUT })

    await row.getByRole('button', { name: 'Comment actions' }).click()
    await ownerPage.getByRole('menuitem', { name: 'Edit' }).click()
    const editor = row.getByRole('textbox')
    await editor.fill('First comment from the owner (edited)')
    await row.getByRole('button', { name: 'Save' }).click()

    await expect(row).toContainText('First comment from the owner (edited)', {
      timeout: WIDGET_TIMEOUT,
    })
    await expect(row).toContainText('(edited)', { timeout: WIDGET_TIMEOUT })
  })

  test('popup shows all comments with working "Load more" past 5', async () => {
    // Seed enough extra comments (via API) to exceed the widget's 5-row cap.
    for (let i = 1; i <= 5; i++) {
      await postJson(ownerPage, `/api/v1/${teamId}/comments`, {
        resource_type: 'artifact',
        resource_id: artifactId,
        content: `Seeded comment number ${i}`,
      })
    }
    await ownerPage.goto(artifactUrl)
    const panel = await commentsPanel(ownerPage)

    // Footer appears only once total > 5 (1 authored + 5 seeded = 6).
    const seeAll = panel.getByTestId('comments-see-all')
    await expect(seeAll).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await expect(seeAll).toContainText('6', { timeout: WIDGET_TIMEOUT })

    await seeAll.click()
    const dialog = ownerPage.getByRole('dialog', { name: /comments \(6\)/i })
    await expect(dialog).toBeVisible({ timeout: WIDGET_TIMEOUT })
    // First page shows 5; "Load more" reveals the 6th, then disappears.
    await expect(dialog.getByTestId('comment-row')).toHaveCount(5, {
      timeout: WIDGET_TIMEOUT,
    })
    await dialog.getByRole('button', { name: 'Load more comments' }).click()
    await expect(dialog.getByTestId('comment-row')).toHaveCount(6, {
      timeout: WIDGET_TIMEOUT,
    })
    await expect(
      dialog.getByRole('button', { name: 'Load more comments' })
    ).toHaveCount(0, { timeout: WIDGET_TIMEOUT })
  })

  test('member sees no actions on the owner’s comment; owner can delete the member’s comment', async () => {
    // Member: no ⋯ menu on someone else's comment. (After seeding, the widget's
    // 5 newest rows are the seeded owner comments; the edited one is now oldest
    // and lives only in the popup, so target a seeded row that's actually shown.)
    await memberPage.goto(artifactUrl)
    const memberPanel = await commentsPanel(memberPage)
    const ownersRow = memberPanel
      .getByTestId('comment-row')
      .filter({ hasText: 'Seeded comment number 5' })
    await expect(ownersRow).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await expect(
      ownersRow.getByRole('button', { name: 'Comment actions' })
    ).toHaveCount(0, { timeout: WIDGET_TIMEOUT })

    // Member adds their own comment (members may comment).
    await memberPanel.getByTestId('comment-add-button').click()
    await memberPanel.getByRole('textbox').fill('A note from the member')
    await memberPanel
      .getByRole('button', { name: 'Comment', exact: true })
      .click()
    await expect(
      memberPanel
        .getByTestId('comment-row')
        .filter({ hasText: 'A note from the member' })
    ).toBeVisible({ timeout: WIDGET_TIMEOUT })

    // Owner: on the member's comment the ⋯ menu offers Delete but NOT Edit
    // (moderation via resource.delete.any; edit is author-only).
    await ownerPage.goto(artifactUrl)
    const ownerPanel = await commentsPanel(ownerPage)
    const membersRow = ownerPanel
      .getByTestId('comment-row')
      .filter({ hasText: 'A note from the member' })
    await expect(membersRow).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await membersRow.getByRole('button', { name: 'Comment actions' }).click()
    await expect(
      ownerPage.getByRole('menuitem', { name: 'Delete' })
    ).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await expect(ownerPage.getByRole('menuitem', { name: 'Edit' })).toHaveCount(
      0,
      { timeout: WIDGET_TIMEOUT }
    )
    await ownerPage.getByRole('menuitem', { name: 'Delete' }).click()

    const confirm = ownerPage.getByRole('alertdialog')
    await expect(confirm).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await confirm.getByRole('button', { name: 'Delete' }).click()
    await expect(confirm).not.toBeVisible({ timeout: WIDGET_TIMEOUT })
    await expect(
      ownerPanel
        .getByTestId('comment-row')
        .filter({ hasText: 'A note from the member' })
    ).toHaveCount(0, { timeout: WIDGET_TIMEOUT })
  })

  test('homepage "Recent comments" card links to the resource detail page', async () => {
    await ownerPage.goto('/')
    await ownerPage.waitForLoadState('networkidle')
    // CardTitle renders a div, not a heading — match by text.
    await expect(
      ownerPage.getByText('Recent comments', { exact: true })
    ).toBeVisible({ timeout: 20000 })

    // Scope to a recent-comments row (the "commented on" phrase is unique to
    // this card) for our artifact — the title also appears in other Home cards.
    const row = ownerPage
      .getByRole('link')
      .filter({ hasText: artifactTitle })
      .filter({ hasText: /commented on|edited a comment on/ })
      .first()
    await expect(row).toBeVisible({ timeout: WIDGET_TIMEOUT })
    await row.click()
    await expect(ownerPage).toHaveURL(new RegExp(`/artifacts/${projectId}/`), {
      timeout: WIDGET_TIMEOUT,
    })
    await expect(ownerPage.getByTestId('comments-panel')).toBeVisible({
      timeout: WIDGET_TIMEOUT,
    })
  })

  test('deleting the resource removes its comments from the homepage card', async () => {
    const del = await ownerPage.request.delete(
      `/api/v1/${teamId}/artifacts/${projectId}/${encodeURIComponent(
        artifactUrl.split('/').pop()!
      )}`
    )
    expect(del.ok(), `delete artifact failed: ${del.status()}`).toBeTruthy()

    await ownerPage.goto('/')
    await ownerPage.waitForLoadState('networkidle')
    await expect(
      ownerPage.getByText('Recent comments', { exact: true })
    ).toBeVisible({ timeout: 20000 })
    // No orphan recent-comment rows for the now-deleted artifact (cascade).
    await expect(
      ownerPage
        .getByRole('link')
        .filter({ hasText: artifactTitle })
        .filter({ hasText: /commented on|edited a comment on/ })
    ).toHaveCount(0, { timeout: WIDGET_TIMEOUT })
  })
})
