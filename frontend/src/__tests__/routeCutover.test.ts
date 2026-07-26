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
import { matchRoutes } from 'react-router-dom'

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
 * A path literal that STARTS at `/settings/<retired>`.
 *
 * Two delimiter families, because routes are written two ways:
 *
 *  - **String literals** — `navigate('/settings/teams')`, `to={\`/settings/x\`}`,
 *    `path="/settings/teams/:id"`. Preceded by a quote, backtick or `(`.
 *  - **Regex literals** — `waitForURL(/\\/settings\\/teams\\/[0-9a-f-]+$/)`, which
 *    Playwright specs use for URL assertions. Here the slashes are ESCAPED, so
 *    the text reads `settings\\/teams` and the string-literal pattern never
 *    matches it.
 *
 * That second case is not hypothetical: #596 was exactly this shape. #538's
 * path replacement missed two regex literals in a Playwright spec, the original
 * version of this guard missed them too, and the stale assertion only surfaced
 * when the e2e suite was finally run by hand.
 *
 * Either way `/teams/x/settings/search` and `@/pages/teams/settings/search`
 * cannot match, because both require the literal to begin at `/settings/`.
 */
const SEGMENTS = RETIRED_SEGMENTS.join('|')
const RETIRED_ROUTE = new RegExp(
  [
    // string literal: '/settings/teams', `/settings/teams/...`, "/settings/..."
    `['"\`(]/settings/(${SEGMENTS})(?=['"\`/?#]|$)`,
    // regex literal with escaped slashes: /settings\/teams\/...$/
    `\\(/(?:\\\\/)?settings\\\\/(${SEGMENTS})(?=\\\\/|['"\`/?#$]|$)`,
  ].join('|')
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
      // Regex literals - the #596 shape the first version of this guard missed.
      'waitForURL(/settings\\/teams\\/[0-9a-f-]+$/)',
      'toHaveURL(/settings\\/teams$/)',
      'expect(page).toHaveURL(/\\/settings\\/projects$/)',
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
      // Regex literals for RETAINED routes and for the new nested tree.
      'toHaveURL(/settings\\/api-keys$/)',
      'toHaveURL(/\\/teams\\/[0-9a-f-]+\\/settings\\/search$/)',
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

/**
 * The guard above proves nothing *links to* a retired path. This one proves
 * what happens when someone arrives at one anyway (#593).
 *
 * `routes.tsx` used to carry `<Route path="settings/*" element={<ComingSoon
 * title="Settings" />} />`, which React Router ranks above the global `*` →
 * `NotFound`. Every retired `/settings/**` path therefore rendered "Settings —
 * coming soon": the one description that is definitely false, since those pages
 * exist under `/teams/:id/**`.
 *
 * Structural rather than a render test, for the reason in this file's header
 * and in `adminBranch.test.ts`: mounting `AppRoutes` needs stubs for ~40 pages,
 * and a hand-built equivalent tree would prove React Router's ranking rather
 * than what `routes.tsx` actually declares.
 */
describe('no `settings/*` catch-all shadows NotFound (#593)', () => {
  const routesSource = readFileSync(join(ROOT, 'src', 'routes.tsx'), 'utf8')

  it('declares no settings wildcard route', () => {
    // Quote-agnostic and whitespace-tolerant, so a reformat or a switch to
    // single quotes cannot smuggle the route back past this guard.
    expect(routesSource).not.toMatch(/path\s*=\s*['"`]settings\/\*['"`]/)
  })

  it('still declares every retained personal settings route', () => {
    // The other half of the assertion: deleting the wildcard must not turn into
    // deleting the pages that legitimately live under /settings (#543).
    for (const retained of RETAINED) {
      expect(routesSource).toMatch(
        new RegExp(`path\\s*=\\s*['"\`]settings/${retained}['"\`]`)
      )
    }
  })

  it('still routes unmatched paths to NotFound', () => {
    // Deleting the wildcard only helps if the global fallback is still there.
    expect(routesSource).toMatch(
      /path\s*=\s*['"`]\*['"`]\s+element\s*=\s*\{\s*<NotFound\s*\/>\s*\}/
    )
  })

  /**
   * The assertions above prove what `routes.tsx` DECLARES. This block proves
   * what those declarations RESOLVE to, by running React Router's real ranking
   * over the real path list.
   *
   * It is not the circular "equivalent tree" test the issue rejected: the paths
   * are scraped out of `routes.tsx` itself, so deleting a retained route or
   * re-adding the wildcard changes this input and flips the expectations. And
   * it costs nothing — `matchRoutes` needs paths only, so none of the ~40 page
   * components has to be imported or stubbed.
   *
   * `AppRoutes` is a single flat `<Routes>` with no nested `<Route>` children
   * (asserted below), which is what makes the scrape sound.
   */
  describe('React Router resolves the retired paths onto the fallback', () => {
    const declaredPaths = [...routesSource.matchAll(/\bpath="([^"]+)"/g)].map(
      m => m[1]
    )
    const routeTable = declaredPaths.map(path => ({ path }))

    /** The path that actually wins for `pathname`, per React Router. */
    const resolve = (pathname: string): string | undefined =>
      matchRoutes(routeTable, pathname)?.at(-1)?.route.path

    it('scraped a plausible route table from a flat <Routes>', () => {
      // Guards the guard: a broken scrape would make every expectation vacuous.
      expect(declaredPaths.length).toBeGreaterThan(30)
      expect(declaredPaths).toContain('*')
      expect(routesSource).not.toContain('</Route>')
    })

    // Exactly the table from the issue body.
    it.each([
      '/settings/search',
      '/settings/model-providers',
      '/settings/embedding-providers',
      '/settings/customization',
      '/settings/integrations/github',
      '/settings/projects',
      '/settings/projects/some-slug',
      '/settings/teams',
      '/settings/teams/team-a',
    ])('%s falls through to the NotFound catch-all', pathname => {
      expect(resolve(pathname)).toBe('*')
    })

    it.each([
      ['/settings', 'settings'],
      ['/settings/activities', 'settings/activities'],
      ['/settings/notifications', 'settings/notifications'],
      ['/settings/api-keys', 'settings/api-keys'],
    ])('%s still resolves to its own route', (pathname, expected) => {
      expect(resolve(pathname)).toBe(expected)
    })

    it('leaves the remaining ComingSoon catch-all alone', () => {
      // It covers a genuinely unbuilt page and is explicitly out of scope.
      expect(resolve('/mcp-servers/anything')).toBe('mcp-servers/*')
    })

    // Epic #610 removed the AI Tools section (#615). Its ComingSoon catch-all went
    // with it, so these must reach NotFound rather than render a "coming soon"
    // placeholder for a feature that no longer exists — the same bug #593 fixed
    // for the retired /settings paths.
    it.each([
      '/ai-tools/overview',
      '/ai-tools/claude-code/overview',
      '/ai-tools/cursor-ide/overview',
      '/ai-tools/claude-code/sessions',
      '/ai-tools/anything',
    ])('%s falls through to the NotFound catch-all', pathname => {
      expect(resolve(pathname)).toBe('*')
    })

    it('still routes team-scoped paths into the team shell', () => {
      expect(resolve('/teams/team-a/settings/search')).toBe('teams/:id/*')
    })
  })
})
