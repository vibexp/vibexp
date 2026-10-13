/**
 * Every relative time ("3h ago") must carry its absolute date-time on hover.
 * `RelativeTime` is the one place that pairs the two, so a component that
 * prints `formatRelativeTime(...)` straight into JSX has dropped the tooltip.
 * jsdom cannot hover, and one missing `title` among dozens of timestamps is
 * invisible in review — so the invariant is pinned on the source instead.
 */
import { readdirSync, readFileSync, statSync } from 'fs'
import { join, relative } from 'path'

const SRC = join(process.cwd(), 'src')
const SKIP_DIRS = new Set(['__tests__', 'node_modules'])
/** The component that renders the pair, and so the one allowed call site. */
const ALLOWED = new Set(['components/RelativeTime.tsx'])

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (full.endsWith('.tsx') && !full.endsWith('.test.tsx'))
      out.push(full)
  }
  return out
}

describe('relative times carry their absolute date-time', () => {
  it('renders none through a bare formatRelativeTime() in JSX', () => {
    const offenders = walk(SRC)
      .map(file => relative(SRC, file))
      .filter(file => !ALLOWED.has(file))
      .filter(file =>
        /\{\s*formatRelativeTime\(/.test(readFileSync(join(SRC, file), 'utf8'))
      )

    expect(offenders).toEqual([])
  })
})
