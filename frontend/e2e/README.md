# E2E Tests with Playwright

This directory contains end-to-end tests for the VibeXP frontend using Playwright.

## Setup

Playwright is already installed as a dev dependency. To install browsers:

```bash
npx playwright install
```

## Running Tests

### Production-like stack via `make` (recommended)

The supported way to run the suite — locally and in CI — is the `make e2e`
target. It builds the **combined image from source** (the Go backend serving the
embedded SPA + API on one port) alongside Postgres and a `fake-gcs-server`
emulator (so attachment uploads work without real GCS), runs the whole suite,
then tears the stack down:

```bash
# From the repo root. Installs the Playwright browser, builds + boots the stack
# (docker-compose.e2e.yml), runs the suite against http://localhost:8080, then
# always tears the stack down and propagates the suite's exit code.
make e2e
```

Granular targets (useful for iterating — bring the stack up once, run repeatedly):

```bash
make e2e-up        # build + start the stack, wait until healthy
make e2e-test      # run the suite against the running stack (re-runnable)
make e2e-down      # stop the stack and wipe its volumes
make e2e-browsers  # install the Playwright chromium browser only
```

Why a combined image + emulator: the suite runs against the artifact we actually
ship, the `fake-gcs-server` gives the backend a credential-free object store so
attachment tests pass, and the stack raises the per-IP auth rate limiter so a
whole suite logging in from one IP is never throttled (429). All of these values
are throwaway/test-only — never reuse `docker-compose.e2e.yml` for a real deploy.

**Always use `CI=true`** (the make targets set it) when running against the
stack: it stops Playwright from trying to start its own Vite dev server, which
would conflict with the container. The base URL is `http://localhost:8080`
(override with `PLAYWRIGHT_BASE_URL` / `E2E_BASE_URL`).

### On demand in CI (`workflow_dispatch`)

The suite is **not** wired to every PR/push (it builds an image and boots a full
stack — too heavy to gate every PR). Run it manually from the **Actions → CI /
E2E** workflow (`.github/workflows/ci-e2e.yml`), which takes a `branch` input so
you can run it against any branch from anywhere:

```bash
gh workflow run ci-e2e.yml -f branch=<your-branch>
```

It delegates entirely to `make e2e`, so a green run there means the same
`make e2e` is green locally. The Playwright HTML report + traces/videos are
uploaded as a build artifact.

### Quick iteration against the Vite dev server

For fast local iteration you can also run the suite against `make
frontend-run-dev` (Vite on :5173). Playwright auto-starts the dev server when
`CI` is unset. Note: attachment tests need the object store, and the dev backend
must have the auth rate limiter raised — prefer `make e2e` for a faithful run.

```bash
npm run test:e2e          # headless
npm run test:e2e:ui       # interactive UI mode
npm run test:e2e:headed   # see the browser
npm run test:e2e:debug    # step-through debugger
npm run test:e2e:report   # open the last HTML report
```

## Test Structure

### Auth Fixture (`fixtures/auth.ts`)

Provides reusable authentication utilities:

- **`authenticatedPage` fixture**: Automatically logs in before each test
- **`devLogin(page, email?, name?)`**: Helper function for manual login
- **`isAuthenticated(page)`**: Check if user is authenticated
- **`logout(page)`**: Helper to logout

### Using the Auth Fixture

#### Example 1: Using the `authenticatedPage` fixture

```typescript
import { test, expect } from './fixtures/auth'

test('my test', async ({ authenticatedPage }) => {
  // Already logged in!
  await authenticatedPage.goto('/prompts')
  // ... test logic
})
```

#### Example 2: Manual login with devLogin helper

```typescript
import { test, expect } from '@playwright/test'
import { devLogin } from './fixtures/auth'

test('my test', async ({ page }) => {
  await devLogin(page)
  // Now authenticated
  await page.goto('/dashboard')
  // ... test logic
})
```

#### Example 3: Custom email for specific tests

```typescript
import { devLogin } from './fixtures/auth'

test('specific user test', async ({ page }) => {
  await devLogin(
    page,
    'playwright_vibexp_specific@example.com',
    'Specific User'
  )
  // ... test logic
})
```

### Feature Coverage (`features/`)

Each app area has a focused spec directory under `e2e/features/`:

| Area                            | Spec                                  | Data strategy                                                                     |
| ------------------------------- | ------------------------------------- | --------------------------------------------------------------------------------- |
| `prompts`, `artifacts`, `teams` | CRUD + filtering                      | Seeded through the UI                                                             |
| `blueprints`                    | `blueprints/blueprint-crud.spec.ts`   | Seeded through the UI (project picked from the team's default project)            |
| `feeds`                         | `feeds/feed-crud.spec.ts`             | Feeds + feed items seeded through the UI (post composer)                          |
| `showcase`                      | `showcase/showcase.spec.ts`           | Static component gallery — no API data                                            |
| `agents`                        | `agents/agent-pages.spec.ts`          | **Route mocks** (`mockAgentsApi`) — agents register from live agent-card URLs     |
| `ai-tools`                      | `ai-tools/ai-tools.spec.ts`           | **Route mocks** (`mockAiToolsApi`) — session data is ingested from real installs  |
| `mcp-servers`                   | `mcp-servers/mcp-servers.spec.ts`     | Real backend (team context only)                                                  |
| `notifications`                 | `notifications/notifications.spec.ts` | **Route mocks** (`mockNotificationsApi`, stateful) + real backend for preferences |

### API Mocking Conventions (`fixtures/api-mocks.ts`)

Use route mocks only for data that cannot be seeded through the UI (agents,
ai-tools sessions, notifications). Conventions:

- One `page.route()` per API surface that dispatches on the URL path, so
  sub-endpoint registration order never matters; unmatched requests
  `route.continue()` to the real backend.
- Mock the **wire shape** exactly: team-scoped agent endpoints return the
  `{ status, message, data }` envelope, executions/conversations return bare
  pages, notifications follow the OpenAPI `NotificationListResponse`.
- `mockNotificationsApi` keeps mutable in-memory state so mark-as-read flows
  behave like the real backend.
- Build payloads with the `buildMock*` helpers instead of inline objects.

## Test User Naming Convention

All Playwright test users use emails in the format:

```
playwright_vibexp{timestamp}@example.com
```

This makes it easy to identify and clean up test data in the database.

## Dev Login Feature

The dev login feature is only available in development mode. Tests use the query parameter `?dev_login=true` to ensure the dev login form is visible during tests.

## Writing New Tests

1. Import the auth fixture: `import { test, expect } from './fixtures/auth'`
2. Use `authenticatedPage` if you need authentication
3. Follow existing test patterns in `auth.spec.ts`
4. Use descriptive test names and organize into test suites with `test.describe()`

## Best Practices

- ✅ Use unique emails for each test run (timestamp-based)
- ✅ Use the `authenticatedPage` fixture for authenticated tests
- ✅ Clean up test data if needed (logout after tests)
- ✅ Use descriptive test names
- ✅ Group related tests with `test.describe()`
- ✅ Add comments for complex test logic

## CI/CD Integration

The tests are configured to:

- Retry failed tests 2 times in CI
- Run in parallel locally, sequentially in CI
- Capture screenshots and videos on failure
- Generate HTML reports

## Environment Variables

- `PLAYWRIGHT_BASE_URL`: Base URL for the application (default: http://localhost:5173)

## Troubleshooting

### Tests fail with "Element not found"

- Ensure the dev server is running
- Check if selectors have changed in the UI
- Use `test:e2e:headed` to see what's happening

### Authentication issues

- Verify backend is running and accessible
- Check if dev login is enabled in the environment
- Verify the `/api/v1/auth/dev/login` endpoint is working

### Flaky tests

- Add appropriate `waitFor` calls
- Use Playwright's auto-waiting features
- Check for race conditions in async operations
