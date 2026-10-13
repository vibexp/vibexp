import { expect, type Page } from '@playwright/test'

const TAXONOMY_HEADING = 'Labels & metadata'

/** The details column's section headings, in document order. */
async function columnHeadings(page: Page): Promise<string[]> {
  const headings = page.getByTestId('details-column').locator('h3')
  await expect(headings.first()).toBeVisible({ timeout: 10000 })
  return (await headings.allTextContents()).map(text => text.trim())
}

/**
 * From a resource's reading page (#1180): opens Edit, checks the edit column
 * carries the view column's sections — every heading it has is one the reading
 * page has, in the same order — then attaches `filePath` and removes it again
 * without leaving the edit page.
 *
 * The edit column is a subset: Relations and Comments are not edited there, so
 * the reading page has them between Metadata and Attachments and the edit page
 * does not.
 *
 * Attachments are not form state: they persist the moment they upload, which
 * is exactly why they can be managed here at all.
 */
export async function attachWhileEditing(
  page: Page,
  editButtonTestId: string,
  filePath: string,
  fileName: string
): Promise<void> {
  const viewHeadings = await columnHeadings(page)

  await page.getByTestId(editButtonTestId).click()
  await expect(page).toHaveURL(/\/edit$/, { timeout: 10000 })
  await expect(
    page.getByRole('button', { name: 'Save changes' }).first()
  ).toBeVisible({ timeout: 10000 })

  const editHeadings = await columnHeadings(page)
  expect(editHeadings).toEqual(['Metadata', 'Labels & metadata', 'Attachments'])
  // The reading page drops Labels & metadata while there is nothing to show
  // (a fresh resource has no labels); the edit page always has it, because
  // that is where labels are added. Every other heading must line up.
  const shared = (headings: string[]) =>
    headings.filter(heading => heading !== TAXONOMY_HEADING)
  const editShared = shared(editHeadings)
  const viewShared = shared(viewHeadings)
  expect(viewShared).toEqual(expect.arrayContaining(editShared))
  expect(viewShared.filter(heading => editShared.includes(heading))).toEqual(
    editShared
  )

  const card = page.getByTestId('attachment-card')
  await expect(card.getByText('No attachments yet.')).toBeVisible({
    timeout: 10000,
  })
  const items = page.getByTestId('attachment-item')
  await page
    .locator('input[aria-label="Upload attachment"]')
    .setInputFiles(filePath)
  await expect(items.filter({ hasText: fileName })).toBeVisible({
    timeout: 10000,
  })

  const row = items.filter({ hasText: fileName })
  await row.hover()
  await row.getByRole('button', { name: `Delete ${fileName}` }).click()
  await expect(items).toHaveCount(0, { timeout: 10000 })
  await expect(card.getByText('No attachments yet.')).toBeVisible()
}
