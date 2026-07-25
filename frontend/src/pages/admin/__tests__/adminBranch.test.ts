/**
 * Structural guardrails for the decoupled admin branch (#456).
 *
 * `adminRouting.test.tsx` mounts an equivalent tree, which proves the routes and
 * the shell work but cannot prove that `App.tsx` wires them the same way — a
 * mis-ordered or re-nested branch would keep every render test green while
 * putting `/admin` back inside the team-scoped shell in production. These
 * assertions read the actual source, in the spirit of the backend's call-graph
 * guardrails.
 */
import { readFileSync } from 'fs'
import { join } from 'path'

const src = (relative: string) =>
  readFileSync(join(__dirname, '..', '..', '..', relative), 'utf8')

const appSource = src('App.tsx')
const routesSource = src('routes.tsx')

it('mounts /admin/* before the catch-all that renders MainApp', () => {
  const adminAt = appSource.indexOf('path="/admin/*"')
  const catchAllAt = appSource.indexOf('path="/*"')

  expect(adminAt).toBeGreaterThan(-1)
  expect(catchAllAt).toBeGreaterThan(-1)
  // react-router picks the best match rather than the first, but ordering is
  // what makes the intent legible — and a `/admin` (no `/*`) mount point would
  // break every nested admin path.
  expect(adminAt).toBeLessThan(catchAllAt)
})

it('renders the admin branch with no team or project provider', () => {
  const adminApp = appSource.slice(
    appSource.indexOf('function AdminApp('),
    appSource.indexOf('function App(')
  )

  expect(adminApp).toContain('AdminShell')
  expect(adminApp).toContain('RequireInstanceAdmin')
  // The whole point of #456: instance-scoped pages must not sit under
  // team-scoped context, and must not reuse the team-scoped Layout.
  expect(adminApp).not.toContain('TeamProvider')
  expect(adminApp).not.toContain('ProjectProvider')
  expect(adminApp).not.toContain('<Layout')
})

it('keeps the admin branch out of the team-scoped app routes', () => {
  // A stray admin import here is the tell-tale of a re-nested admin route.
  expect(routesSource).not.toMatch(/from '@\/pages\/admin\//)
  expect(routesSource).not.toContain('path="admin"')
})

it('has no remaining reference to the retired AdminLayout', () => {
  expect(() => src('pages/admin/AdminLayout.tsx')).toThrow()
  expect(appSource).not.toContain('AdminLayout')
  expect(routesSource).not.toContain('AdminLayout')
})
