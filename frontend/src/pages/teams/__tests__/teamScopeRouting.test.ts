/**
 * Structural guardrails for the team-scope subtree (#539).
 *
 * `TeamScopeLayout.test.tsx` mounts an equivalent tree, which proves the layout
 * and its routes work but cannot prove that `routes.tsx` wires them the same way
 * in production. Specifically: a bare `teams/:id` mount (no `/*`) renders the
 * layout fine for `/teams/:id` and silently 404s every nested path, while every
 * render test stays green. These assertions read the actual source.
 *
 * Same pattern and rationale as `pages/admin/__tests__/adminBranch.test.ts`,
 * kept in its own file so the admin guardrails stay about the admin branch.
 */
import { readFileSync } from 'fs'
import { join } from 'path'

const routesSource = readFileSync(
  join(__dirname, '..', '..', '..', 'routes.tsx'),
  'utf8'
)

it('mounts the team scope as a splat route', () => {
  // The `/*` is what lets the nested <Routes> in TeamRoutes match children.
  expect(routesSource).toContain('path="teams/:id/*"')
  expect(routesSource).toContain('<TeamScopeLayout />')
})

it('does not also mount the old flat team detail/analytics routes', () => {
  // Leaving these behind would shadow the scoped subtree and skip the
  // membership gate and the tab bar entirely.
  expect(routesSource).not.toContain('path="teams/:id"')
  expect(routesSource).not.toContain('path="teams/:id/analytics"')
})

it('keeps the teams list outside the scoped subtree', () => {
  // `/teams` is not team-scoped - there is no `:id` to resolve - so it must not
  // sit under TeamScopeLayout.
  expect(routesSource).toContain('path="teams" element={<Teams />}')
})

it('mounts the team scope before the app catch-all', () => {
  const teamScopeAt = routesSource.indexOf('path="teams/:id/*"')
  const appCatchAll = routesSource.indexOf('path="*"')

  expect(teamScopeAt).toBeGreaterThan(-1)
  // react-router picks the best match rather than the first, but ordering is
  // what keeps the intent legible.
  expect(teamScopeAt).toBeLessThan(appCatchAll)
})

it('has no settings catch-all left to order against', () => {
  // This assertion used to read `indexOf('path="settings/*"')` and compare it to
  // the team-scope mount. #593 deleted that route so retired /settings paths
  // reach NotFound, which turned the lookup into -1 and made the comparison
  // pass or fail for the wrong reason. The fact worth keeping is simply that the
  // route is gone; `routeCutover.test.ts` owns the full guard.
  expect(routesSource).not.toContain('path="settings/*"')
})

it('routes the team pages through TeamRoutes, not routes.tsx directly', () => {
  // After #539 these are children of the scope, so importing them here again
  // is the tell-tale of a re-flattened route.
  expect(routesSource).not.toMatch(/from '@\/pages\/teams\/TeamDetailsPage'/)
  expect(routesSource).not.toMatch(/from '@\/pages\/teams\/TeamAnalyticsPage'/)
})
