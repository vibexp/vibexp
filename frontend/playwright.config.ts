import { defineConfig, devices } from '@playwright/test'

/*
 * Journeys that change INSTANCE-WIDE state (#1567): the admin settings every
 * team and every sign-in reads (auth allowlist and providers, the email
 * provider, the audit-log retention, the search defaults, the site info).
 * They run in their own project, one at a time, so no other spec runs while
 * the instance is reconfigured under it; in CI they get a stack of their own.
 * A journey that writes /admin/settings belongs here (the password journeys
 * have their own project below). Everything else must touch only the users
 * and teams it creates, which is what lets the other projects run in parallel.
 */
const INSTANCE_JOURNEYS = [
  '**/journeys/admin-auth-settings.journey.spec.ts',
  '**/journeys/admin-email-settings.journey.spec.ts',
  '**/journeys/audit-log.journey.spec.ts',
  '**/journeys/instance-settings.journey.spec.ts',
  '**/journeys/site-info.journey.spec.ts',
]

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e',

  /* Global timeout. Keep at the Playwright default (30s): the auth fixture's
   * internal waits (login redirect + team-context hydration, up to 15s each)
   * don't fit in a 15s test budget on a cold start or a loaded CI runner. */
  timeout: 30000,

  /* Run tests in files in parallel */
  fullyParallel: true,

  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,

  /* Retry on CI only */
  retries: process.env.CI ? 2 : 0,

  /* Sequential execution for journey tests reliability - 1 worker locally, 2 on CI */
  workers: process.env.CI ? 2 : 1,

  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  /* A sharded CI run (#1562) writes a blob report per shard, which the
   * workflow's report job merges into one HTML + JSON report. */
  reporter: process.env.PLAYWRIGHT_BLOB_REPORT
    ? [['list'], ['blob']]
    : process.env.CI
      ? [
          ['list'],
          ['html'],
          ['json', { outputFile: 'test-results/results.json' }],
        ]
      : [
          ['html', { outputFolder: 'playwright-report' }],
          ['json', { outputFile: 'test-results/results.json' }],
        ],

  /* Output folder for test results */
  outputDir: 'test-results',

  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Base URL to use in actions like `await page.goto('/')`. */
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173',

    /* Viewport size */
    viewport: { width: 1920, height: 1080 },

    /* Grant clipboard permissions for copy/paste tests */
    permissions: ['clipboard-read', 'clipboard-write'],

    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'on-first-retry',

    /* Screenshot on failure */
    screenshot: 'only-on-failure',

    /* Video on failure */
    video: 'retain-on-failure',
  },

  /* Configure projects for major browsers */
  projects: [
    {
      name: 'chromium',
      // Only run the top-level specs here; smoke/journey/feature files have
      // dedicated projects below. Without this ignore every one of those tests
      // ran TWICE per CI run (once under chromium, once under its own
      // project), doubling runtime and cross-test load (a flakiness source).
      testIgnore: [
        '**/*.smoke.spec.ts',
        '**/*.journey.spec.ts',
        '**/features/**',
      ],
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 }, // Override with our custom viewport
      },
    },

    /* Smoke tests - fast critical path validation for CI gating */
    {
      name: 'smoke',
      testMatch: '**/*.smoke.spec.ts',
      retries: 0, // No retries for smoke tests - they should be stable
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
      },
    },

    /* Journey tests - end-to-end user journey scenarios */
    {
      name: 'journeys',
      testMatch: '**/*.journey.spec.ts',
      // The instance-wide and password journeys have their own projects below.
      testIgnore: ['**/password-*.journey.spec.ts', ...INSTANCE_JOURNEYS],
      retries: process.env.CI ? 2 : 1, // Allow retries for journey tests
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
      },
    },

    /* Instance-wide journeys (#1567): see INSTANCE_JOURNEYS. After the
     * parallel projects, one at a time. Alone:
     * `npx playwright test --project=instance-journeys --no-deps [file]`. */
    {
      name: 'instance-journeys',
      testMatch: INSTANCE_JOURNEYS,
      dependencies: ['chromium', 'smoke', 'journeys', 'features'],
      fullyParallel: false,
      workers: 1,
      retries: process.env.CI ? 2 : 1,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
      },
    },

    /*
     * Password journeys (#1444): they switch password sign-in, the registration
     * mode and the instance email provider, which the instance-wide journeys
     * also touch (admin-auth-settings, admin-email-settings). So they run LAST,
     * after those too, one at a time, and put every setting back. Through the project dependencies,
     * a test that still fails in an earlier project means these do not run at
     * all (reported as "did not run"). To run them on their own, or one of
     * them: `npx playwright test --project=password-journeys --no-deps [file]`.
     */
    {
      name: 'password-journeys',
      testMatch: '**/password-*.journey.spec.ts',
      dependencies: ['instance-journeys'],
      fullyParallel: false,
      workers: 1,
      retries: process.env.CI ? 2 : 1,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
      },
    },

    /* Feature tests - focused feature testing */
    {
      name: 'features',
      testMatch: 'e2e/features/**/*.spec.ts',
      retries: process.env.CI ? 2 : 1, // Allow retries for feature tests
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1920, height: 1080 },
      },
    },
  ],

  /* Run your local dev server before starting the tests (only for local development) */
  webServer: process.env.CI
    ? undefined
    : {
        command: 'npm run dev',
        url: 'http://localhost:5173',
        reuseExistingServer: true,
        timeout: 120000,
      },
})
