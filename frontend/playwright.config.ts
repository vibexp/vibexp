import { defineConfig, devices } from '@playwright/test'

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
  reporter: process.env.CI
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
      retries: process.env.CI ? 2 : 1, // Allow retries for journey tests
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
