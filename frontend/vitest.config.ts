import path from 'node:path'
import { defineConfig } from 'vitest/config'

// Test-runner config (issue #498: jest/ts-jest -> Vitest). Kept separate from
// vite.config.ts so the build config stays free of test-only aliasing; the `@`
// alias below mirrors vite.config.ts's resolve.alias exactly.
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      // Static asset + ESM-only-dep stubs, ported from jest's moduleNameMapper.
      // lucide-react is a behavioural icon allowlist, not an ESM shim — it
      // stays mocked under Vitest.
      'lucide-react': path.resolve(__dirname, './tests/mocks/lucide-react.tsx'),
      marked: path.resolve(__dirname, './tests/mocks/marked.js'),
      mermaid: path.resolve(__dirname, './tests/mocks/mermaid.js'),
      // The generated API client is stubbed for tests; type-only imports are
      // erased — tests mock @/lib/apiClientGenerated for behaviour.
      '@vibexp/api-client': path.resolve(
        __dirname,
        './tests/mocks/vibexpApiClient.ts'
      ),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}', 'src/**/*.test.{ts,tsx}'],
    fakeTimers: {
      // Under jest, @testing-library's waitFor automatically used fake
      // (jasmine) timers; under Vitest it defaults to real ones, so a
      // debounce-advanced-by-fake-timers + waitFor test hangs to the 5s
      // timeout. ShouldAdvanceTime restores the jest behaviour.
      shouldAdvanceTime: true,
    },
    env: {
      // jest.config.js used to seed these via `globals['import.meta']`;
      // src/utils/environment.ts still reads the test branch from process.env
      // (same process under Vitest's jsdom environment).
      VITE_API_BASE_URL: 'https://api.vibexp.io/api/v1',
      VITE_GTM_ENABLED: 'false',
      VITE_GTM_ID: '',
      VITE_GA4_MEASUREMENT_ID: '',
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov', 'html'],
      reportsDirectory: 'coverage',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.d.ts', 'src/main.tsx', 'src/vite-env.d.ts'],
    },
  },
})
