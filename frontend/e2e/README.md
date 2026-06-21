# E2E Tests with Playwright

This directory contains end-to-end tests for the VibeXP frontend using Playwright.

## Setup

Playwright is already installed as a dev dependency. To install browsers:

```bash
npx playwright install
```

## Running Tests

### Local Development (with Vite dev server)

```bash
# Run all e2e tests in headless mode
npm run test:e2e

# Run tests with UI (interactive mode)
npm run test:e2e:ui

# Run tests in headed mode (see browser)
npm run test:e2e:headed

# Debug tests
npm run test:e2e:debug

# View test report after running tests
npm run test:e2e:report
```

### Local E2E with Docker Compose (Production-like)

Run tests against production builds using docker-compose (same as CI):

```bash
# From project root directory
cd /path/to/vibexp.io

# Start all services (postgres, backend, frontend)
docker compose -f docker-compose.e2e.yml up -d

# Wait for services to be healthy (check with: docker compose -f docker-compose.e2e.yml ps)
# Once all are healthy, run tests from frontend directory with CI flag
cd frontend
CI=true npm run test:e2e

# View logs if needed
cd ..
docker compose -f docker-compose.e2e.yml logs

# Stop and clean up
docker compose -f docker-compose.e2e.yml down -v
```

**Important:** Always use `CI=true` when running E2E tests with Docker Compose. This prevents Playwright from trying to start a local dev server which would conflict with the Docker containers.

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
