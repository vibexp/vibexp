import { test, expect } from '../../fixtures/auth'
import { selectFirstProject } from '../../helpers/artifacts'

/**
 * Feature Test: mermaid diagram label rendering (#744)
 *
 * This is the ONLY layer that can catch this class of bug. `vitest.config.ts`
 * aliases `mermaid` to a stub, so no unit test ever sees real mermaid output,
 * and jsdom cannot drive mermaid at all (it dies on `CSSStyleSheet is not
 * defined`). A regression here is therefore invisible to `make frontend-test`.
 *
 * Three stacked defects had to be fixed before a diagram could render its
 * labels here, and this spec guards all three end to end:
 *   1. the ```mermaid fence was matched against marked's OUTPUT, where it is
 *      already `<pre><code class="language-mermaid">` — so nothing was ever
 *      extracted and every diagram degraded to a code block;
 *   2. `MermaidDiagram` mounted its ref'd container only on success, so the
 *      render effect bailed on a null ref and spun forever;
 *   3. mermaid emitted labels as HTML inside `<foreignObject>`, whose contents
 *      the SVG-only DOMPurify profile strips — fixed with `htmlLabels: false`,
 *      which keeps labels as native SVG `<text>`.
 *
 * Note (3) alone is what the issue described, on the assumption that shapes and
 * arrows already rendered. They did not: (1) meant nothing rendered at all.
 *
 * Assertions are on `<text>` content specifically, not on `body` text: the
 * label text must survive sanitization *inside the SVG*, which is exactly what
 * the bug broke.
 */

const DETAIL_URL = /artifacts\/[^/]+\/[^/]+/

const FLOWCHART = `# Diagram

\`\`\`mermaid
flowchart TD
    A[Start] --> B{Decision}
    B -->|yes| C[Do the thing]
    B -->|no| D[Stop]
\`\`\`
`

// A node label carrying an XSS payload. mermaid's own `securityLevel: 'strict'`
// plus our sanitize step must render it inert.
const XSS_FLOWCHART = `# Probe

\`\`\`mermaid
flowchart TD
    N1["<img src=x onerror=window.__XSS_FIRED__=true>"] --> N2[Safe]
\`\`\`
`

async function createArtifactWithContent(
  page: import('@playwright/test').Page,
  slugPrefix: string,
  content: string
): Promise<void> {
  await page.goto('/artifacts/new')
  await expect(page).toHaveURL(/artifacts\/new/)

  const stamp = Date.now()
  await page.waitForSelector('[data-testid="artifact-project-select"]', {
    timeout: 10000,
  })
  await page
    .locator('[data-testid="artifact-slug-input"]')
    .fill(`${slugPrefix}-${String(stamp)}`)
  await page
    .locator('[data-testid="artifact-title-input"]')
    .fill(`Mermaid ${slugPrefix} ${String(stamp)}`)
  await page.locator('[data-testid="artifact-content-textarea"]').fill(content)
  await selectFirstProject(page)
  await page.locator('button:has-text("Create Artifact")').click()

  await expect(page).toHaveURL(DETAIL_URL, { timeout: 10000 })
}

test.describe('Mermaid diagram rendering', () => {
  test('flowchart node and edge labels render as SVG text', async ({
    authenticatedPage: page,
  }) => {
    await createArtifactWithContent(page, 'flowchart', FLOWCHART)

    // The diagram renders asynchronously into a placeholder div.
    const svg = page.locator('.mermaid-container svg')
    await expect(svg).toBeVisible({ timeout: 15000 })

    // Node labels — these were the blank ones.
    for (const label of ['Start', 'Decision', 'Do the thing', 'Stop']) {
      await expect(
        svg.locator('text', { hasText: new RegExp(`^${label}$`) })
      ).toHaveCount(1)
    }

    // Edge labels.
    for (const label of ['yes', 'no']) {
      await expect(
        svg.locator('text', { hasText: new RegExp(`^${label}$`) })
      ).toHaveCount(1)
    }

    // The fix's mechanism: no foreignObject means nothing for the SVG-only
    // sanitize profile to strip. Asserting this pins the *cause*, so a future
    // config change that reverts to HTML labels fails here even if some other
    // change happened to keep the text visible.
    await expect(svg.locator('foreignObject')).toHaveCount(0)
  })

  test('an XSS payload in a node label is inert', async ({
    authenticatedPage: page,
  }) => {
    await createArtifactWithContent(page, 'xss', XSS_FLOWCHART)

    const svg = page.locator('.mermaid-container svg')
    await expect(svg).toBeVisible({ timeout: 15000 })

    // The diagram still rendered (positive gate: without this, the negative
    // assertion below would pass on a page that rendered nothing at all).
    await expect(svg.locator('text', { hasText: /^Safe$/ })).toHaveCount(1)

    // No <img> element was created, and the handler never ran.
    await expect(svg.locator('img')).toHaveCount(0)
    const fired = await page.evaluate(
      () => (window as unknown as Record<string, unknown>).__XSS_FIRED__
    )
    expect(fired).toBeUndefined()
  })
})
