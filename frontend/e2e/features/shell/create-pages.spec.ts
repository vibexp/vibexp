import { expect, test } from '../../fixtures/auth'

/**
 * The four create pages render in the reading shell (#1181), so creating a
 * resource looks like editing it and reading it: the name is the inline
 * header, Create/Cancel are reading actions, and the fields sit in the details
 * column — or, on a phone, the chips under the title and the details sheet.
 *
 * The per-kind create-and-redirect flows live in each kind's own CRUD spec;
 * this one pins the layout they all share.
 */

const CREATE_PAGES = [
  {
    path: '/artifacts/new',
    singular: 'artifact',
    name: 'artifact-title-input',
  },
  {
    path: '/blueprints/new',
    singular: 'blueprint',
    name: 'blueprint-title-input',
  },
  { path: '/memories/new', singular: 'memory', name: 'memory-title-input' },
  { path: '/prompts/new', singular: 'prompt', name: 'prompt-name-input' },
] as const

test.describe('Create pages in the reading shell', () => {
  for (const { path, singular, name } of CREATE_PAGES) {
    test(`${path} renders the editing reading page`, async ({
      authenticatedPage: page,
    }) => {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto(path)

      const shell = page.getByTestId('reading-page')
      await expect(shell).toHaveAttribute('data-presentation', 'editing')
      await expect(shell.getByTestId('resource-form')).toBeVisible()

      // The name is the header, as on edit — a memory's short title
      // included, since its content is the body.
      const header = shell.locator('article header')
      await expect(header.getByTestId(name)).toHaveAttribute(
        'placeholder',
        `Untitled ${singular}`
      )

      const column = page.getByTestId('details-column')
      await expect(
        column.getByRole('button', { name: `Create ${singular}` })
      ).toBeVisible()
      await expect(column.getByRole('button', { name: 'Cancel' })).toBeVisible()
    })
  }

  test('typing a title fills the slug field and the header chip', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/blueprints/new')

    await page.getByTestId('blueprint-title-input').fill('Shell Create Check')
    await expect(page.getByTestId('blueprint-slug-input')).toHaveValue(
      'shell-create-check'
    )
    await expect(
      page.locator('article header').getByText('shell-create-check')
    ).toBeVisible()
  })

  test('on a phone, Create and Cancel are chips under the title and the details open as a sheet', async ({
    authenticatedPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/blueprints/new')

    const header = page.locator('article header')
    const chips = header.getByTestId('reading-actions-chips')
    await expect(
      chips.getByRole('button', { name: 'Create blueprint' })
    ).toBeVisible()
    await expect(chips.getByRole('button', { name: 'Cancel' })).toBeVisible()
    // Under the title, not beside it.
    const titleBox = await header
      .getByTestId('blueprint-title-input')
      .boundingBox()
    const chipsBox = await chips.boundingBox()
    if (!titleBox || !chipsBox) throw new Error('header has no layout')
    expect(chipsBox.y).toBeGreaterThanOrEqual(titleBox.y + titleBox.height)

    // No details column at this width: the fields are one tap away.
    await expect(page.getByTestId('details-column')).toHaveCount(0)
    await page.getByTestId('details-toggle').click()
    const sheet = page.getByTestId('reading-details-sheet')
    await expect(sheet).toBeVisible()
    await expect(sheet.getByTestId('blueprint-project-select')).toBeVisible()
  })
})
