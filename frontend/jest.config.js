export default {
  preset: 'ts-jest',
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: ['<rootDir>/tests/setup.ts'],
  moduleNameMapper: {
    // Static asset stubs MUST come before the `@/` path alias so that
    // `@/assets/foo.svg` is mapped to the stub instead of being resolved as an
    // actual file (jest applies the first matching pattern).
    '\\.(png|jpg|jpeg|gif|webp|svg)$': '<rootDir>/tests/mocks/fileMock.js',
    '\\.(css|less|scss|sass)$': 'identity-obj-proxy',
    '^@/(.*)$': '<rootDir>/src/$1',
    '^lucide-react$': '<rootDir>/tests/mocks/lucide-react.tsx',
    '^marked$': '<rootDir>/tests/mocks/marked.js',
    '^mermaid$': '<rootDir>/tests/mocks/mermaid.js',
    '^../utils/environment$': '<rootDir>/tests/mocks/environment.ts',
    '^../../src/utils/environment$': '<rootDir>/tests/mocks/environment.ts',
    // Generated API client is ESM only; stub it for jest/CJS. Type-only
    // imports are erased — tests mock @/lib/apiClientGenerated for behavior.
    '^@vibexp/api-client$': '<rootDir>/tests/mocks/vibexpApiClient.ts',
  },
  transform: {
    '^.+\\.tsx?$': [
      'ts-jest',
      {
        tsconfig: 'jest.tsconfig.json',
        // import.meta is invalid in ts-jest's CommonJS output. The transformer
        // below rewrites it to globalThis["import.meta"] (provided via the
        // `globals` config); ignore the now-moot TS1343 type diagnostic.
        diagnostics: { ignoreCodes: [1343] },
        astTransformers: {
          before: ['<rootDir>/tests/transformers/importMeta.cjs'],
        },
      },
    ],
  },
  transformIgnorePatterns: [
    'node_modules/(?!(marked|prismjs|mermaid|@marked)/)',
  ],
  testMatch: [
    '<rootDir>/tests/**/*.(test|spec).(ts|tsx|js)',
    '<rootDir>/src/**/*.(test|spec).(ts|tsx|js)',
  ],
  collectCoverageFrom: [
    'src/**/*.{ts,tsx}',
    '!src/**/*.d.ts',
    '!src/main.tsx',
    '!src/vite-env.d.ts',
  ],
  coverageDirectory: 'coverage',
  coverageReporters: ['text', 'lcov', 'html'],
  globals: {
    'import.meta': {
      env: {
        DEV: false,
        VITE_API_BASE_URL: 'https://api.vibexp.io/api/v1',
        VITE_GTM_ENABLED: 'false',
        VITE_GTM_ID: '',
        VITE_GA4_MEASUREMENT_ID: '',
      },
    },
  },
}
