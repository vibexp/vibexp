import { test, expect, type Page } from '../../fixtures/auth'
import { generateFeedData } from '../../fixtures/test-data'

/**
 * Feature Tests: AI Feeds CRUD
 *
 * Covers the feeds list (/feeds), feed creation (/feeds/new), the feed view
 * with its post composer (/feeds/:feedId), feed editing (/feeds/:feedId/edit),
 * and the feed item detail page (/feed-items/:itemId). Feed items are seeded
 * through the UI via the post composer ("user post"), not via API mocks.
 */

/** Create a feed via the UI; resolves on the feed view page. */
async function createFeed(page: Page, feed: { name: string }): Promise<void> {
  await page.goto('/feeds/new')
  await expect(page.getByLabel('Name')).toBeVisible({ timeout: 10000 })

  await page.getByLabel('Name').fill(feed.name)
  await page.getByRole('button', { name: 'Create feed' }).click()

  await page.waitForURL(/\/feeds\/[^/]+$/, { timeout: 15000 })
}

/** Post a feed item through the composer on the current feed view page. */
async function postFeedItem(
  page: Page,
  item: { title: string; content: string }
): Promise<void> {
  // The composer collapses to a single "Share an update…" input; focusing it
  // expands the title + content fields.
  const titleInput = page.getByRole('textbox', { name: 'Post title' })
  await titleInput.click()
  await titleInput.fill(item.title)
  await page.getByRole('textbox', { name: 'Post content' }).fill(item.content)
  await page.getByRole('button', { name: 'Post', exact: true }).click()

  // The new item shows up in the feed's item list.
  await expect(page.getByText(item.title).first()).toBeVisible({
    timeout: 10000,
  })
}

test.describe('Feed CRUD Operations', () => {
  test('should display the feeds page', async ({ authenticatedPage }) => {
    await authenticatedPage.goto('/feeds')
    await expect(authenticatedPage).toHaveURL(/feeds$/)

    await expect(
      authenticatedPage.getByRole('heading', { name: 'AI Feeds' })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByRole('button', { name: 'New feed' })
    ).toBeVisible()
  })

  test('should show empty state for a fresh user', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/feeds')
    await expect(authenticatedPage.getByText('No feed items yet')).toBeVisible({
      timeout: 10000,
    })
  })

  test('should create a feed and land on its view', async ({
    authenticatedPage,
  }) => {
    const feed = generateFeedData()
    await createFeed(authenticatedPage, feed)

    await expect(
      authenticatedPage.getByRole('heading', { name: feed.name })
    ).toBeVisible({ timeout: 10000 })
    await expect(
      authenticatedPage.getByRole('button', { name: 'Edit feed' })
    ).toBeVisible()
  })

  test('should validate required name on create', async ({
    authenticatedPage,
  }) => {
    await authenticatedPage.goto('/feeds/new')
    await expect(authenticatedPage.getByLabel('Name')).toBeVisible({
      timeout: 10000,
    })

    await authenticatedPage.getByRole('button', { name: 'Create feed' }).click()

    await expect(authenticatedPage.getByText('Name is required')).toBeVisible({
      timeout: 5000,
    })
    await expect(authenticatedPage).toHaveURL(/feeds\/new/)
  })

  test('should edit a feed name', async ({ authenticatedPage }) => {
    const feed = generateFeedData()
    await createFeed(authenticatedPage, feed)

    await authenticatedPage.getByRole('button', { name: 'Edit feed' }).click()
    await expect(authenticatedPage).toHaveURL(/feeds\/[^/]+\/edit$/, {
      timeout: 10000,
    })

    const updatedName = `${feed.name} (Updated)`
    const nameInput = authenticatedPage.getByLabel('Name')
    await expect(nameInput).toHaveValue(feed.name, { timeout: 10000 })
    await nameInput.fill(updatedName)

    await authenticatedPage
      .getByRole('button', { name: 'Save changes' })
      .click()

    await authenticatedPage.waitForURL(/\/feeds\/[^/]+$/, { timeout: 15000 })
    await expect(
      authenticatedPage.getByRole('heading', { name: updatedName })
    ).toBeVisible({ timeout: 10000 })
  })

  test('should post a feed item via the composer', async ({
    authenticatedPage,
  }) => {
    const feed = generateFeedData()
    await createFeed(authenticatedPage, feed)

    const itemTitle = `E2E update ${Date.now()}`
    await postFeedItem(authenticatedPage, {
      title: itemTitle,
      content: 'Posted from the Playwright feed CRUD spec.',
    })
  })

  test('should open a feed item detail page', async ({ authenticatedPage }) => {
    const feed = generateFeedData()
    await createFeed(authenticatedPage, feed)

    const itemTitle = `E2E item detail ${Date.now()}`
    const itemContent = 'Feed item content rendered on the detail page.'
    await postFeedItem(authenticatedPage, {
      title: itemTitle,
      content: itemContent,
    })

    await authenticatedPage
      .getByRole('link', { name: `Open feed item: ${itemTitle}` })
      .click()

    await authenticatedPage.waitForURL(/\/feed-items\/[^/]+$/, {
      timeout: 15000,
    })
    await expect(authenticatedPage.getByText(itemTitle).first()).toBeVisible({
      timeout: 10000,
    })
    await expect(authenticatedPage.getByText(itemContent).first()).toBeVisible()
  })

  test('should show the new feed item on the all-feeds page', async ({
    authenticatedPage,
  }) => {
    const feed = generateFeedData()
    await createFeed(authenticatedPage, feed)

    const itemTitle = `E2E aggregated ${Date.now()}`
    await postFeedItem(authenticatedPage, {
      title: itemTitle,
      content: 'Item visible on the aggregated /feeds page.',
    })

    await authenticatedPage.goto('/feeds')
    await expect(authenticatedPage.getByText(itemTitle).first()).toBeVisible({
      timeout: 10000,
    })
  })
})
