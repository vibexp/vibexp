/**
 * Structural guard against reintroducing a retired `/settings/*` frontend route
 * (epic #536).
 *
 * Same idea as `pages/admin/__tests__/adminBranch.test.ts`: read the actual
 * source and assert a structural fact that render tests cannot prove. A
 * component test happily mounts `<ProjectDetails />` under any path you hand
 * it, so a stale `/settings/projects/:slug` fixture stays green forever while
 * the real route is gone.
 *
 * It rides the normal `make frontend-test` target, so CI enforces it with no
 * workflow change.
 *
 * ## Why the matching is anchored rather than a substring
 *
 * The retired segments still appear all over the tree in shapes that are
 * entirely correct:
 *
 *   - the new routes nest them: `/teams/team-a/settings/search`
 *   - imports mirror the directory move: `@/pages/teams/settings/search/...`
 *   - API paths are unchanged and unrelated: `/api/v1/settings/api-keys`,
 *     `/api/v1/{team_id}/settings/search`
 *
 * So a bare `includes('/settings/search')` would flag dozens of correct lines,
 * and a guard that cries wolf gets deleted. The match therefore requires the
 * literal to *begin* at `/settings/`, i.e. be preceded by a quote, backtick or
 * `(` — which is exactly the shape of a router path or a `navigate()`/`Link`
 * target, and never the shape of a nested route or an import specifier.
 */
import { readdirSync, readFileSync, statSync } from 'fs'
import { join, relative } from 'path'

/** Frontend routes retired by epic #536. */
const RETIRED_SEGMENTS = [
  'teams',
  'projects',
  'search',
  'customization',
  'model-providers',
  'embedding-providers',
  'integrations',
]

/**
 * Retained personal routes — deliberately NOT in the list above.
 *
 * They survived the epic because each is user-scoped at the API layer
 * (`/api/v1/activities`, `/api/v1/preferences`, `/api/v1/settings/api-keys`),
 * so they belong on the personal hub. See #543.
 */
const RETAINED = ['activities', 'notifications', 'api-keys']

/**
 * A path literal that STARTS at `/settings/<retired>` — quote/backtick/paren
 * delimited, so `/teams/x/settings/search` and `@/pages/teams/settings/search`
 * cannot match.
 */
const RETIRED_ROUTE = new RegExp(
  `['"\`(]/settings/(${RETIRED_SEGMENTS.join('|')})(?=['"\`/?#]|$)`
)

/**
 * Files that reference a retired route ON PURPOSE, each with its reason.
 * Anything added here needs a comment justifying it — the point of the guard is
 * that the list stays short and deliberate.
 */
const ALLOWED = new Map<string, string>([
  [
    'src/components/layout/__tests__/HeaderBreadcrumb.test.tsx',
    'asserts the retired /settings/teams path does NOT resolve to "Teams"',
  ],
  [
    'src/components/layout/__tests__/nav-items.test.ts',
    'asserts no nav href lives under the retired /settings/teams path',
  ],
  [
    'src/pages/teams/settings/integrations/github/__tests__/GitHubIntegration.test.tsx',
    'mounts the page at arbitrary paths on purpose (#485): the callback strips ' +
      'params at whatever path it is mounted at, and the legacy path is one of ' +
      'the fixtures proving that location-independence',
  ],
  [
    'src/__tests__/routeCutover.test.ts',
    'this guard names the retired routes it looks for',
  ],
])

const ROOT = join(__dirname, '..', '..')
const SCANNED_DIRS = ['src', 'e2e']
const SKIP_DIRS = new Set(['node_modules', 'dist', 'coverage', '.git'])
const EXTENSIONS = ['.ts', '.tsx']

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      walk(full, out)
    } else if (EXTENSIONS.some(ext => entry.endsWith(ext))) {
      out.push(full)
    }
  }
  return out
}

function offendingLines(file: string): string[] {
  return readFileSync(file, 'utf8')
    .split('\n')
    .map((line, i) => ({ line, n: i + 1 }))
    .filter(({ line }) => RETIRED_ROUTE.test(line))
    .map(({ line, n }) => `${n}: ${line.trim()}`)
}

describe('retired /settings/* frontend routes stay retired (#545)', () => {
  // e2e is in scope deliberately: those specs navigate to literal URLs, so a
  // stale one there is exactly as broken as a stale one in src/ - and it fails
  // only when someone actually runs e2e, which PR CI does not.
  const files = SCANNED_DIRS.flatMap(d => walk(join(ROOT, d)))

  it('scans a meaningful number of files', () => {
    // Guards the guard: a broken walk silently passes everything.
    expect(files.length).toBeGreaterThan(200)
  })

  it('finds no reference to a retired route', () => {
    const offenders = files
      .map(file => ({
        file: relative(ROOT, file),
        lines: offendingLines(file),
      }))
      .filter(({ file, lines }) => lines.length > 0 && !ALLOWED.has(file))

    expect(
      offenders.map(o => `${o.file}\n    ${o.lines.join('\n    ')}`)
    ).toEqual([])
  })

  describe('the matcher itself', () => {
    it.each([
      "navigate('/settings/projects')",
      'to={`/settings/projects/${slug}`}',
      `path="/settings/teams/:id"`,
      "goto('/settings/search')",
      "initialEntries={['/settings/integrations/github']}",
    ])('flags %s', line => {
      expect(RETIRED_ROUTE.test(line)).toBe(true)
    })

    it.each([
      // The new nested routes - the single biggest false-positive risk.
      'navigate(`/teams/${id}/settings/search`)',
      "href: '/teams/team-a/settings/model-providers'",
      "renderAt('/teams/team-a/settings/customization')",
      // Import specifiers mirroring the directory move.
      "import { SearchSettings } from '@/pages/teams/settings/search/SearchSettings'",
      "jest.mock('@/pages/teams/settings/customization/Customization')",
      // API paths - unchanged by this epic and none of its business.
      "get('/api/v1/settings/api-keys')",
      'get(`/api/v1/${teamId}/settings/search`)',
      // Retained personal routes.
      "navigate('/settings/activities')",
      "navigate('/settings/notifications')",
      "navigate('/settings/api-keys')",
      // Near-misses that must not over-match.
      "navigate('/settingsfoo')",
      "navigate('/settings')",
    ])('does not flag %s', line => {
      expect(RETIRED_ROUTE.test(line)).toBe(false)
    })

    it('keeps the retained routes out of the retired list', () => {
      for (const retained of RETAINED) {
        expect(RETIRED_SEGMENTS).not.toContain(retained)
      }
    })
  })
})
