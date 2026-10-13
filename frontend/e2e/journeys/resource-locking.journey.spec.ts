import {
  expect,
  test,
  type APIResponse,
  type Browser,
  type BrowserContext,
  type Locator,
  type Page,
} from '@playwright/test'

import { devLogin } from '../fixtures/auth'
import { closeMoreActions, openMoreActions } from '../helpers/readingPage'

/**
 * Resource locking, end to end (issue #1294, the ship gate for epic #1285).
 *
 *   member A locks an artifact from its detail page, with a comment (#1291)
 *     -> the Metadata section names the locker without a reload, and the
 *        comment tooltip is anchored on its trigger
 *     -> member B, who CREATED the artifact, is offered neither Unlock nor
 *        Delete, is refused both by the server, and edits its content while
 *        the frozen fields are disabled (#1289/#1293)
 *     -> the list marks the row and the "Locked only" filter finds it (#1292)
 *     -> deleting the containing project is refused (#1290)
 *     -> the team owner unlocks it, the history shows both events, and Delete
 *        works again
 *     -> a shorter pass for a prompt and a blueprint: lock, refused delete,
 *        unlock by the locker, delete.
 *
 * ## Why these assertions need an e2e
 *
 * Every sub-issue of the epic is tested with a mock at its boundary. What none
 * of them can show is one lock seen from three sessions with three different
 * sets of rights: placed by one member, respected in another member's pages
 * and requests, and released by someone who did not place it. The tooltip's
 * position is the other reason: jsdom has no layout engine, so the "trigger
 * lost its box and the bubble went to the viewport origin" regression (#891)
 * is invisible to the unit suite.
 *
 * ## How this stays deterministic
 *
 * Nothing waits on wall-clock time. The team is new, so it holds exactly the
 * resources seeded here. Each negative has a positive beside it that would
 * fail if the page or the rule were simply absent: B owns both artifacts and
 * is offered Lock and Delete on the unlocked control, so their absence on the
 * locked one is about the lock; the control row is what the list filter has to
 * drop; and the project delete that was refused goes through once nothing in
 * it is locked. Scaffolding (the team, the project, the invitations, the
 * resources) goes through the API because it is not what is under test;
 * locking, unlocking, editing, filtering and the final delete are driven
 * through the pages.
 */

const stamp = Date.now()
const worker = process.env.TEST_WORKER_INDEX ?? '0'

const OWNER_NAME = 'Locking Owner'
const LOCKER_NAME = 'Locking Alice'
const MEMBER_NAME = 'Locking Bob'

const LOCK_COMMENT = 'Read by the nightly release agent'
const EDITED_CONTENT = 'Rewritten by the second member while locked.'

/** Combined-stack UI budget, matching the other journeys. */
const UI_TIMEOUT = 20_000

/**
 * How far the tooltip may sit from the trigger it labels, edge to edge. The
 * bubble's own offset is 4px; the rest is slack for sub-pixel layout. Against
 * the #891 regression the gap is hundreds of pixels, so this discriminates.
 */
const ANCHOR_TOLERANCE = 12

/**
 * Everything a locator matches by NAME is scoped per ATTEMPT, not per file:
 * `describe.serial` re-runs `beforeAll` on a retry, and a file-scoped name
 * would match the previous attempt's rows too. Assigned in `beforeAll`.
 */
let ARTIFACT_TITLE: string
let ARTIFACT_SLUG: string
let CONTROL_TITLE: string
let CONTROL_SLUG: string
let PROMPT_SLUG: string
let BLUEPRINT_SLUG: string
let PROJECT_SLUG: string

let ownerCtx: BrowserContext
let lockerCtx: BrowserContext
let memberCtx: BrowserContext
let ownerPage: Page
let lockerPage: Page
let memberPage: Page

let teamId: string
let projectId: string
let artifactId: string
let ownerId: string
let lockerId: string

type Json = Record<string, unknown>

interface LockPayload {
  comment: string
  locked_by: { user_id: string; display_name: string }
}

interface LockEvent {
  action: 'lock' | 'unlock'
  actor: { user_id: string } | null
  comment: string | null
}

/** An API call that must succeed; returns the parsed JSON body. */
async function api(
  page: Page,
  method: 'GET' | 'POST',
  url: string,
  data?: unknown
): Promise<Json> {
  const res = await page.request.fetch(url, { method, data })
  const body = await res.text()
  expect(
    res.ok(),
    `${method} ${url} failed: ${String(res.status())} ${body}`
  ).toBe(true)
  return JSON.parse(body) as Json
}

/** The id of the user signed in on `page`. */
async function userIdOf(page: Page): Promise<string> {
  const me = await api(page, 'GET', '/api/v1/auth/me')
  const id = ((me.user as Json | undefined)?.id ?? me.id) as string
  expect(id, 'no user id on /auth/me').toBeTruthy()
  return id
}

/**
 * A second or third actor: invited as a plain member, signed in in a context
 * of their own, and joined through the API — joining a team is not what is
 * under test. Asserts the role the journey's permission checks rest on.
 */
async function joinAsMember(
  browser: Browser,
  email: string,
  name: string
): Promise<{ ctx: BrowserContext; page: Page }> {
  await api(ownerPage, 'POST', `/api/v1/teams/${teamId}/invitations`, {
    emails: [email],
    role: 'member',
  })
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  await devLogin(page, email, name)

  const { invitations } = (await api(
    page,
    'GET',
    '/api/v1/invitations/pending'
  )) as { invitations?: { token: string }[] }
  const token = invitations?.[0]?.token ?? ''
  expect(token, `${name} has no pending invitation`).toBeTruthy()
  await api(
    page,
    'POST',
    `/api/v1/invitations/${encodeURIComponent(token)}/accept`
  )

  const { teams } = (await api(page, 'GET', '/api/v1/teams')) as {
    teams: { id: string; role: string; permissions: string[] }[]
  }
  const joined = teams.find(candidate => candidate.id === teamId)
  expect(joined?.role, `${name} did not join as a plain member`).toBe('member')
  // The reason this member may not unlock someone else's lock.
  expect(joined?.permissions).not.toContain('resource.unlock.any')
  return { ctx, page }
}

/** The three lockable kinds, with where each lives in the API and the SPA. */
const KINDS = {
  artifact: {
    apiPath: (slug: string) =>
      `/api/v1/${teamId}/artifacts/${projectId}/${slug}`,
    pagePath: (slug: string) => `/artifacts/${projectId}/${slug}`,
  },
  blueprint: {
    apiPath: (slug: string) =>
      `/api/v1/${teamId}/blueprints/${projectId}/${slug}`,
    pagePath: (slug: string) => `/blueprints/${projectId}/${slug}`,
  },
  prompt: {
    apiPath: (slug: string) => `/api/v1/${teamId}/prompts/${slug}`,
    pagePath: (slug: string) => `/prompts/${slug}`,
  },
} as const

type Kind = keyof typeof KINDS

/** The lock the API stores on a resource, or `null` when it has none. */
async function storedLock(
  page: Page,
  kind: Kind,
  slug: string
): Promise<LockPayload | null> {
  const resource = await api(page, 'GET', KINDS[kind].apiPath(slug))
  // `lock` is a required key: an absent one is a contract break, not "unlocked".
  expect(resource).toHaveProperty('lock')
  return resource.lock as LockPayload | null
}

/** The resource's lock history as the API returns it: newest first. */
async function storedHistory(kind: Kind, slug: string): Promise<LockEvent[]> {
  const history = await api(
    ownerPage,
    'GET',
    `${KINDS[kind].apiPath(slug)}/lock/history`
  )
  return history.events as LockEvent[]
}

/** Asserts a refused write: `423 Locked` carrying the given problem code. */
async function expectLocked(res: APIResponse, code: string): Promise<Json> {
  const text = await res.text()
  expect(res.status(), text).toBe(423)
  const problem = JSON.parse(text) as Json
  expect(problem.code).toBe(code)
  return problem
}

/**
 * Open a resource's detail page and wait for its loaded action row — the Edit
 * button is offered to every member and only once the resource has loaded, so
 * an absence asserted after this is a real absence, not a page still loading.
 */
async function openDetail(page: Page, kind: Kind, slug: string): Promise<void> {
  await page.goto(KINDS[kind].pagePath(slug))
  await expect(page.getByTestId(`edit-${kind}-button`)).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

/** Lock the resource on the open detail page through the lock dialog. */
async function lockFromDetailPage(page: Page, kind: Kind): Promise<void> {
  // Lock is an item of the reading toolbar's More menu.
  await openMoreActions(page)
  await page.getByTestId(`lock-${kind}-button`).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText(`Lock ${kind}?`)
  // The comment is required: nothing can be submitted until it is typed.
  await expect(dialog.getByTestId('confirm-lock-button')).toBeDisabled()
  await dialog.getByTestId('lock-comment-input').fill(LOCK_COMMENT)
  await dialog.getByTestId('confirm-lock-button').click()
  await expect(dialog).toHaveCount(0, { timeout: UI_TIMEOUT })
}

/** A row of the artifacts list, matched on the artifact's title. */
function artifactRow(page: Page, title: string) {
  return page.getByRole('row').filter({ hasText: title })
}

/** Open the artifacts list and wait until its rows have loaded. */
async function openArtifactList(page: Page): Promise<void> {
  await page.goto('/artifacts')
  await expect(artifactRow(page, CONTROL_TITLE)).toBeVisible({
    timeout: UI_TIMEOUT,
  })
}

interface Box {
  x: number
  y: number
  width: number
  height: number
}

async function boxOf(locator: Locator, what: string): Promise<Box> {
  const box = await locator.boundingBox()
  expect(box, `${what} has no layout box`).not.toBeNull()
  return box as Box
}

/** The shortest distance between two boxes, edge to edge; 0 when they touch. */
function gapBetween(a: Box, b: Box): number {
  const dx = Math.max(a.x - (b.x + b.width), b.x - (a.x + a.width), 0)
  const dy = Math.max(a.y - (b.y + b.height), b.y - (a.y + a.height), 0)
  return Math.hypot(dx, dy)
}

test.describe.serial('Resource locking journey', () => {
  test.describe.configure({ timeout: 180_000 })

  test.beforeAll(async ({ browser }, testInfo) => {
    const run = `${worker}-${String(stamp)}-${String(testInfo.retry)}`
    // Short on purpose: a prompt name is capped at 50 characters.
    const tag = `${worker}${stamp.toString(36)}${String(testInfo.retry)}`
    ARTIFACT_TITLE = `Lock journey artifact ${tag}`
    ARTIFACT_SLUG = `lock-journey-${tag}`
    CONTROL_TITLE = `Lock journey control ${tag}`
    CONTROL_SLUG = `lock-control-${tag}`
    PROMPT_SLUG = `lock-prompt-${tag}`
    BLUEPRINT_SLUG = `lock-blueprint-${tag}`
    PROJECT_SLUG = `lock-project-${tag}`

    ownerCtx = await browser.newContext()
    ownerPage = await ownerCtx.newPage()
    await devLogin(ownerPage, `e2e_lock_owner_${run}@example.com`, OWNER_NAME)
    ownerId = await userIdOf(ownerPage)

    const team = await api(ownerPage, 'POST', '/api/v1/teams', {
      name: `Lock Journey Team ${run}`,
      description: 'Created by the resource locking e2e',
    })
    teamId = team.id as string
    expect(teamId, 'team id missing from create response').toBeTruthy()
    // The owner is the "admin" of the journey: the one actor who may release
    // a lock they did not place.
    expect(team.permissions).toContain('resource.unlock.any')

    const project = await api(ownerPage, 'POST', `/api/v1/${teamId}/projects`, {
      name: `Lock Journey Project ${tag}`,
      slug: PROJECT_SLUG,
    })
    projectId = project.id as string
    expect(projectId, 'project id missing from create response').toBeTruthy()

    const locker = await joinAsMember(
      browser,
      `e2e_lock_alice_${run}@example.com`,
      LOCKER_NAME
    )
    lockerCtx = locker.ctx
    lockerPage = locker.page
    lockerId = await userIdOf(lockerPage)

    const member = await joinAsMember(
      browser,
      `e2e_lock_bob_${run}@example.com`,
      MEMBER_NAME
    )
    memberCtx = member.ctx
    memberPage = member.page

    // B creates both artifacts. A member may delete what they created, so the
    // server refusing B's delete of the locked one — and the page not offering
    // it — can only be about the lock.
    const created = await api(
      memberPage,
      'POST',
      `/api/v1/${teamId}/artifacts`,
      {
        project_id: projectId,
        slug: ARTIFACT_SLUG,
        title: ARTIFACT_TITLE,
        content: 'Seeded for the resource locking journey.',
      }
    )
    artifactId = created.id as string
    await api(memberPage, 'POST', `/api/v1/${teamId}/artifacts`, {
      project_id: projectId,
      slug: CONTROL_SLUG,
      title: CONTROL_TITLE,
      content: 'Never locked: the control for the list and the gating checks.',
    })
    await api(ownerPage, 'POST', `/api/v1/${teamId}/prompts`, {
      project_id: projectId,
      slug: PROMPT_SLUG,
      name: `Lock prompt ${tag}`,
      body: 'Seeded prompt body.',
    })
    await api(ownerPage, 'POST', `/api/v1/${teamId}/blueprints`, {
      project_id: projectId,
      slug: BLUEPRINT_SLUG,
      title: `Lock journey blueprint ${tag}`,
      content: 'Seeded blueprint content.',
    })

    // All three actors work inside the new team. `addInitScript` rather than
    // a post-load localStorage write, which races team hydration.
    for (const page of [ownerPage, lockerPage, memberPage]) {
      await page.addInitScript(id => {
        localStorage.setItem('vx_current_team_id', id)
      }, teamId)
    }
  })

  // Optional chaining is deliberate: a `beforeAll` that fails partway leaves
  // the later contexts unassigned, and an unguarded `close()` would replace
  // the real setup failure with a TypeError.
  test.afterAll(async () => {
    await memberCtx?.close()
    await lockerCtx?.close()
    await ownerCtx?.close()
  })

  test('member A locks the artifact with a comment', async () => {
    await openDetail(lockerPage, 'artifact', ARTIFACT_SLUG)
    const lockedBy = lockerPage.getByTestId('locked-by')
    await expect(lockedBy).toHaveCount(0)
    expect(await storedLock(lockerPage, 'artifact', ARTIFACT_SLUG)).toBeNull()

    await lockFromDetailPage(lockerPage, 'artifact')

    // No reload: the row appears on the page the lock was placed from, and
    // the locker is offered the way back.
    await expect(
      lockerPage
        .getByTestId('metadata-panel')
        .getByText('Locked by', { exact: true })
    ).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(lockedBy).toHaveText(LOCKER_NAME)
    await openMoreActions(lockerPage)
    await expect(lockerPage.getByTestId('lock-artifact-button')).toHaveCount(0)
    await expect(lockerPage.getByTestId('unlock-artifact-button')).toBeVisible()
    await closeMoreActions(lockerPage)

    // What the page shows is what the API stores.
    const lock = await storedLock(lockerPage, 'artifact', ARTIFACT_SLUG)
    expect(lock?.locked_by.user_id).toBe(lockerId)
    expect(lock?.locked_by.display_name).toBe(LOCKER_NAME)
    expect(lock?.comment).toBe(LOCK_COMMENT)
  })

  test('the lock comment tooltip is anchored on its trigger', async () => {
    await openDetail(lockerPage, 'artifact', ARTIFACT_SLUG)
    // An entering tooltip slides and zooms, so a box read mid-flight is a
    // moving position. Zero the durations rather than sleeping; `0s`, not
    // `none`, because Radix unmounts on `animationend`.
    await lockerPage.addStyleTag({
      content:
        '*, *::before, *::after { animation-duration: 0s !important; transition-duration: 0s !important; }',
    })

    const trigger = lockerPage.getByTestId('locked-by')
    await trigger.scrollIntoViewIfNeeded()
    await trigger.hover()

    // The wrapper floating-ui positions, rather than `role="tooltip"`: that
    // role sits on a 1px visually-hidden copy inside the bubble.
    const bubble = lockerPage
      .locator('[data-radix-popper-content-wrapper]')
      .filter({ hasText: LOCK_COMMENT })
    await expect(bubble).toBeVisible()
    await expect(lockerPage.getByRole('tooltip')).toHaveText(LOCK_COMMENT)

    // The trigger must have a box of its own. A `display: contents` wrapper
    // has none, which is exactly what sends the bubble to the viewport origin.
    const triggerBox = await boxOf(trigger, 'the "Locked by" trigger')
    expect(triggerBox.width).toBeGreaterThan(0)
    expect(triggerBox.height).toBeGreaterThan(0)
    // The trigger sits in the details column, far from the viewport origin.
    // That distance is what lets the check below tell an anchored bubble from
    // one parked at the origin; were the trigger up there too, both would pass.
    expect(Math.hypot(triggerBox.x, triggerBox.y)).toBeGreaterThan(
      ANCHOR_TOLERANCE * 10
    )

    // The expected position is derived from the trigger's own box. Polled
    // because the popper places itself in a post-layout effect.
    await expect
      .poll(
        async () => {
          const box = await bubble.boundingBox()
          return box ? gapBetween(box, triggerBox) : Number.POSITIVE_INFINITY
        },
        {
          message:
            'the tooltip never settled beside the "Locked by" trigger — it is anchored on the viewport origin, not on the trigger',
        }
      )
      .toBeLessThanOrEqual(ANCHOR_TOLERANCE)
  })

  test('member B is offered neither Unlock nor Delete, and the server refuses both', async () => {
    // The control first: on an unlocked artifact B created, both actions are
    // on the page. That is what makes their absence below mean something.
    await openDetail(memberPage, 'artifact', CONTROL_SLUG)
    await openMoreActions(memberPage)
    await expect(memberPage.getByTestId('lock-artifact-button')).toBeVisible()
    await expect(memberPage.getByTestId('delete-artifact-button')).toBeVisible()

    await openDetail(memberPage, 'artifact', ARTIFACT_SLUG)
    await expect(memberPage.getByTestId('locked-by')).toHaveText(LOCKER_NAME, {
      timeout: UI_TIMEOUT,
    })
    // Opened first (when there is one): an item missing from a closed menu
    // would prove nothing. With no menu at all there is no such action.
    await openMoreActions(memberPage)
    await expect(memberPage.getByTestId('unlock-artifact-button')).toHaveCount(
      0
    )
    await expect(memberPage.getByTestId('lock-artifact-button')).toHaveCount(0)
    await expect(memberPage.getByTestId('delete-artifact-button')).toHaveCount(
      0
    )

    // Hiding a button is convenience; the server is what refuses. B created
    // this artifact, so the delete is not refused for want of a permission...
    const artifactUrl = KINDS.artifact.apiPath(ARTIFACT_SLUG)
    const refusedDelete = await expectLocked(
      await memberPage.request.delete(artifactUrl),
      'RESOURCE_WRITE_LOCKED'
    )
    expect(refusedDelete.metadata).toMatchObject({
      resource_type: 'artifact',
      comment: LOCK_COMMENT,
      locked_by: { user_id: lockerId },
    })
    // ...while the unlock is: B neither placed the lock nor may release any.
    const refusedUnlock = await memberPage.request.delete(`${artifactUrl}/lock`)
    expect(refusedUnlock.status()).toBe(403)

    const lock = await storedLock(memberPage, 'artifact', ARTIFACT_SLUG)
    expect(lock?.locked_by.user_id).toBe(lockerId)
  })

  test('member B edits the content while the frozen fields are disabled', async () => {
    await openDetail(memberPage, 'artifact', ARTIFACT_SLUG)
    await memberPage.getByTestId('edit-artifact-button').click()
    await expect(memberPage).toHaveURL(/\/edit$/, { timeout: UI_TIMEOUT })

    // The notice says who and why, on the page the fields are frozen on.
    const notice = memberPage.getByTestId('resource-lock-notice')
    await expect(notice).toContainText(
      `Locked by ${LOCKER_NAME}: ${LOCK_COMMENT}`,
      { timeout: UI_TIMEOUT }
    )

    // Status, type, project, labels and metadata: on the page, and disabled.
    for (const testId of [
      'artifact-status-select',
      'artifact-type-select',
      'artifact-project-select',
      'artifact-labels-input',
      'metadata-add-pair',
    ]) {
      await expect(memberPage.getByTestId(testId)).toBeDisabled()
    }

    // The content is not frozen.
    const content = memberPage.getByTestId('artifact-content-textarea')
    await expect(content).toBeEnabled()
    await content.fill(EDITED_CONTENT)
    // By exact name: the edit header's "Copy slug: <slug>" chip would match a
    // loose /update|save/ filter whenever the slug contains the word (#1179).
    await memberPage.getByRole('button', { name: 'Save changes' }).click()

    await expect(memberPage).not.toHaveURL(/\/edit$/, { timeout: UI_TIMEOUT })
    await expect(memberPage.getByTestId('resource-body')).toContainText(
      EDITED_CONTENT,
      { timeout: UI_TIMEOUT }
    )

    // Saved, and the save neither released nor moved the lock.
    const stored = await api(
      memberPage,
      'GET',
      KINDS.artifact.apiPath(ARTIFACT_SLUG)
    )
    expect(stored.content).toBe(EDITED_CONTENT)
    expect((stored.lock as LockPayload | null)?.locked_by.user_id).toBe(
      lockerId
    )
  })

  test('the list marks the locked row and the Locked filter finds it', async () => {
    await openArtifactList(memberPage)
    const lockedRow = artifactRow(memberPage, ARTIFACT_TITLE)
    const controlRow = artifactRow(memberPage, CONTROL_TITLE)

    await expect(
      lockedRow.getByRole('img', { name: `Locked by ${LOCKER_NAME}` })
    ).toBeVisible()
    await expect(lockedRow.getByTestId('lock-indicator')).toHaveCount(1)
    await expect(controlRow.getByTestId('lock-indicator')).toHaveCount(0)
    await expect(memberPage).not.toHaveURL(/[?&]locked=/)

    const filter = memberPage.getByTestId('artifact-lock-filter')
    await filter.click()
    await memberPage
      .getByRole('option', { name: 'Locked only', exact: true })
      .click()

    const filteredUrl = /[?&]locked=locked(&|$)/
    await expect(memberPage).toHaveURL(filteredUrl)
    await expect(controlRow).toHaveCount(0, { timeout: UI_TIMEOUT })
    await expect(lockedRow).toBeVisible()

    await memberPage.reload()
    await expect(lockedRow).toBeVisible({ timeout: UI_TIMEOUT })
    await expect(memberPage).toHaveURL(filteredUrl)
    await expect(filter).toContainText('Locked only')
    await expect(controlRow).toHaveCount(0)
  })

  test('deleting the project that holds the locked artifact is refused', async () => {
    const projectUrl = `/api/v1/${teamId}/projects/${PROJECT_SLUG}`
    // As the owner, who may delete projects: the refusal is not a permission.
    const refused = await expectLocked(
      await ownerPage.request.delete(projectUrl),
      'HAS_LOCKED_RESOURCES'
    )
    const blockers = (refused.metadata as { locked_resources: Json[] })
      .locked_resources
    expect(blockers).toHaveLength(1)
    expect(blockers[0]).toMatchObject({
      resource_type: 'artifact',
      id: artifactId,
    })

    // Nothing was changed: the project and both of its artifacts are there.
    const project = await api(ownerPage, 'GET', projectUrl)
    expect(project.id).toBe(projectId)
    const lock = await storedLock(ownerPage, 'artifact', ARTIFACT_SLUG)
    expect(lock?.locked_by.user_id).toBe(lockerId)
    await api(ownerPage, 'GET', KINDS.artifact.apiPath(CONTROL_SLUG))
  })

  test('the owner unlocks the artifact, the history shows both events, and delete works again', async () => {
    await openDetail(ownerPage, 'artifact', ARTIFACT_SLUG)
    await expect(ownerPage.getByTestId('locked-by')).toHaveText(LOCKER_NAME, {
      timeout: UI_TIMEOUT,
    })
    // Locked: even the owner, who may delete anything, is not offered Delete.
    await openMoreActions(ownerPage)
    await expect(ownerPage.getByTestId('delete-artifact-button')).toHaveCount(0)

    // The owner did not place this lock; `resource.unlock.any` is what puts
    // Unlock on this page and what the server accepts it on.
    await ownerPage.getByTestId('unlock-artifact-button').click()
    await expect(ownerPage.getByTestId('locked-by')).toHaveCount(0, {
      timeout: UI_TIMEOUT,
    })
    await openMoreActions(ownerPage)
    await expect(ownerPage.getByTestId('lock-artifact-button')).toBeVisible()
    await closeMoreActions(ownerPage)
    expect(await storedLock(ownerPage, 'artifact', ARTIFACT_SLUG)).toBeNull()

    // The history, newest first: the owner's unlock, then A's lock with its
    // comment. On the page and in the API.
    const events = ownerPage
      .getByTestId('lock-history')
      .getByTestId('lock-history-event')
    await expect(events).toHaveCount(2, { timeout: UI_TIMEOUT })
    await expect(events.nth(0)).toHaveAttribute('data-action', 'unlock')
    await expect(events.nth(0)).toContainText(`${OWNER_NAME} unlocked`)
    await expect(events.nth(1)).toHaveAttribute('data-action', 'lock')
    await expect(events.nth(1)).toContainText(`${LOCKER_NAME} locked`)
    await expect(events.nth(1)).toContainText(LOCK_COMMENT)

    const history = await storedHistory('artifact', ARTIFACT_SLUG)
    expect(history).toHaveLength(2)
    expect(history[0]).toMatchObject({
      action: 'unlock',
      actor: { user_id: ownerId },
      comment: null,
    })
    expect(history[1]).toMatchObject({
      action: 'lock',
      actor: { user_id: lockerId },
      comment: LOCK_COMMENT,
    })

    // Delete is back, and it deletes.
    await openMoreActions(ownerPage)
    await ownerPage.getByTestId('delete-artifact-button').click()
    const confirm = ownerPage.getByRole('alertdialog')
    await expect(confirm).toContainText('Delete artifact?')
    await confirm.getByRole('button', { name: 'Delete', exact: true }).click()
    await expect(ownerPage).toHaveURL(/\/artifacts$/, { timeout: UI_TIMEOUT })

    const gone = await ownerPage.request.get(
      KINDS.artifact.apiPath(ARTIFACT_SLUG)
    )
    expect(gone.status()).toBe(404)
  })

  // The shorter pass. The unlock here is the LOCKER's own, the path the
  // artifact pass (released by the owner) does not take.
  for (const [kind, slugOf] of [
    ['prompt', () => PROMPT_SLUG],
    ['blueprint', () => BLUEPRINT_SLUG],
  ] as const) {
    test(`a ${kind} is locked, cannot be deleted, and is deletable once unlocked`, async () => {
      const slug = slugOf()
      const resourceUrl = KINDS[kind].apiPath(slug)

      await openDetail(lockerPage, kind, slug)
      await lockFromDetailPage(lockerPage, kind)
      await expect(lockerPage.getByTestId('locked-by')).toHaveText(
        LOCKER_NAME,
        { timeout: UI_TIMEOUT }
      )
      const lock = await storedLock(ownerPage, kind, slug)
      expect(lock?.locked_by.user_id).toBe(lockerId)
      expect(lock?.comment).toBe(LOCK_COMMENT)

      // The page hides Delete on a locked resource, so the refusal is asked
      // of the server directly — as the owner, who may delete anything.
      const refused = await expectLocked(
        await ownerPage.request.delete(resourceUrl),
        'RESOURCE_WRITE_LOCKED'
      )
      expect(refused.metadata).toMatchObject({ resource_type: kind })
      await api(ownerPage, 'GET', resourceUrl)

      await openMoreActions(lockerPage)
      await lockerPage.getByTestId(`unlock-${kind}-button`).click()
      await expect(lockerPage.getByTestId('locked-by')).toHaveCount(0, {
        timeout: UI_TIMEOUT,
      })
      await openMoreActions(lockerPage)
      await expect(lockerPage.getByTestId(`lock-${kind}-button`)).toBeVisible()
      await closeMoreActions(lockerPage)
      expect(await storedLock(ownerPage, kind, slug)).toBeNull()

      // The same request that was refused a moment ago.
      const deleted = await ownerPage.request.delete(resourceUrl)
      expect(deleted.ok(), await deleted.text()).toBe(true)
      expect((await ownerPage.request.get(resourceUrl)).status()).toBe(404)
    })
  }

  test('with nothing left locked, the project delete goes through', async () => {
    // The request `HAS_LOCKED_RESOURCES` refused above. The project still
    // holds the control artifact, so it was the lock, not the contents, that
    // stood in the way.
    const projectUrl = `/api/v1/${teamId}/projects/${PROJECT_SLUG}`
    const deleted = await ownerPage.request.delete(projectUrl)
    expect(deleted.ok(), await deleted.text()).toBe(true)
    expect((await ownerPage.request.get(projectUrl)).status()).toBe(404)
  })
})
