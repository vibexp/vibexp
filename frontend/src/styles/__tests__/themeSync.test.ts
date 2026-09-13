import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

/**
 * The app's `@theme inline` block is a hand-kept copy of the design system's
 * `tokens/theme.css` — Tailwind v4 only scans `@theme` in the ENTRY css file,
 * so importing the package's copy generates no utilities. That copy drifted
 * once already (#921: the brand aliases were missing, so `bg-brand` silently
 * did not exist), and Tailwind reports nothing for an unknown utility.
 *
 * This pins the colour, sizing and radius mappings to upstream. The font,
 * type-scale, weight and tracking mappings are deliberately not mirrored:
 * adopting them would re-point `font-sans` / `text-*` and change rendering.
 */
const MIRRORED_NAMESPACES = ['--color-', '--container-', '--radius-']

function themeDeclarations(css: string): Map<string, string> {
  const block = /@theme inline\s*\{([\s\S]*?)\n\}/.exec(css)
  if (!block) throw new Error('no @theme inline block found')
  const withoutComments = block[1].replace(/\/\*[\s\S]*?\*\//g, '')
  const declarations = new Map<string, string>()
  for (const match of withoutComments.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    declarations.set(match[1], match[2].trim())
  }
  return declarations
}

function read(relative: string): string {
  return readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8')
}

describe('@theme inline sync with @vibexp/design-system', () => {
  const upstream = themeDeclarations(
    read('../../../node_modules/@vibexp/design-system/tokens/theme.css')
  )
  const local = themeDeclarations(read('../index.css'))

  const mirrored = [...upstream].filter(([name]) =>
    MIRRORED_NAMESPACES.some(prefix => name.startsWith(prefix))
  )

  it('finds the upstream mappings it is meant to mirror', () => {
    expect(mirrored.length).toBeGreaterThan(0)
    expect(upstream.has('--color-brand')).toBe(true)
    expect(upstream.has('--container-control-sm')).toBe(true)
  })

  it.each(mirrored)('maps %s exactly as upstream does', (name, value) => {
    expect(local.get(name)).toBe(value)
  })
})
