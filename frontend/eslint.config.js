import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import react from 'eslint-plugin-react'
import tseslint from 'typescript-eslint'
import security from 'eslint-plugin-security'
import sonarjs from 'eslint-plugin-sonarjs'
import jsxA11y from 'eslint-plugin-jsx-a11y'
import { fixupPluginRules } from '@eslint/compat'
import eslintConfigPrettier from 'eslint-config-prettier'
import simpleImportSort from 'eslint-plugin-simple-import-sort'
import unusedImports from 'eslint-plugin-unused-imports'

export default tseslint.config(
  {
    ignores: [
      'dist',
      'node_modules',
      '*.config.js',
      '*.config.ts',
      'coverage',
      '.worktrees',
    ],
  },
  // 1. Base Setup for all files
  {
    // Apply to src files
    files: ['src/**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      // Phase 1: Upgrade to strict type checking
      ...tseslint.configs.strictTypeChecked,
      // Optional: strict stylistic rules (keeps code looking uniform)
      ...tseslint.configs.stylisticTypeChecked,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: {
          jsx: true,
        },
        // Required for strictTypeChecked
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    // Fix compatibility issues with plugins in ESLint 9+
    plugins: {
      'react-hooks': fixupPluginRules(reactHooks),
      'react-refresh': reactRefresh,
      'jsx-a11y': jsxA11y,
      react,
      security,
      sonarjs,
      'simple-import-sort': simpleImportSort,
      'unused-imports': unusedImports,
    },
    settings: {
      react: {
        version: 'detect',
      },
    },
    // 2. Rules Configuration
    rules: {
      // Load standard recommended rules
      ...reactHooks.configs.recommended.rules,
      ...react.configs.recommended.rules,
      ...react.configs['jsx-runtime'].rules,
      ...jsxA11y.flatConfigs.recommended.rules, // Phase 1: Add A11y
      ...security.configs.recommended.rules,

      // React Refresh
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],

      // --- Phase 1: AI Guardrails ---
      // STRICT: No 'any'. This is the most critical AI guardrail.
      '@typescript-eslint/no-explicit-any': 'error',
      // STRICT: No floating promises (missing awaits)
      '@typescript-eslint/no-floating-promises': 'error',

      // TODO: Re-enable after fixing FormEvent deprecations (tracked in separate issue)
      // Updated TypeScript-ESLint 8.56.0 now flags FormEvent as deprecated
      // These are pre-existing code quality issues unrelated to security fixes
      // Note: Requires fixing 9 FormEvent usages across components to SubmitEvent
      '@typescript-eslint/no-deprecated': 'off',

      // --- Storage Access Guardrails ---
      // Prevent direct localStorage/sessionStorage access - use centralized storage utilities
      'no-restricted-globals': [
        'error',
        {
          name: 'localStorage',
          message:
            'Direct localStorage access is not allowed. Use `import { storage } from "@/utils/storage"` instead. See src/constants/storageKeys.ts for available keys.',
        },
        {
          name: 'sessionStorage',
          message:
            'Direct sessionStorage access is not allowed. Use `import { sessionStore } from "@/utils/storage"` instead. See src/constants/storageKeys.ts for available keys.',
        },
      ],

      // --- Import Organization & Cleanup (Phase 1: Warn only) ---
      // NOTE: Set to 'warn' during migration period. Will be set to 'error' after:
      // 1. Removing all lint suppression comments from 57 files
      // 2. Fixing 283 underlying linting issues
      // 3. Running auto-fix across entire codebase
      // TODO: Create follow-up ticket for Phase 2 migration
      'simple-import-sort/imports': 'warn',
      'simple-import-sort/exports': 'warn',

      // CLEANUP: Unused vars and imports
      '@typescript-eslint/no-unused-vars': 'off', // Disabled in favor of unused-imports
      'unused-imports/no-unused-imports': 'warn', // Warn during migration
      'unused-imports/no-unused-vars': [
        'error',
        {
          vars: 'all',
          varsIgnorePattern: '^_',
          args: 'after-used',
          argsIgnorePattern: '^_',
        },
      ],

      // --- Phase 1: Intermediate Complexity Limits ---
      // Reduced from 45 -> 30 (User consensus)
      complexity: ['error', 30],
      // Reduced from 900 -> 600 (User consensus)
      'max-lines': [
        'error',
        { max: 600, skipBlankLines: true, skipComments: true },
      ],
      // Reduced from 850 -> 300 (User consensus)
      'max-lines-per-function': [
        'error',
        { max: 300, skipBlankLines: true, skipComments: true },
      ],
      // SonarJS: Match the standard complexity
      'sonarjs/cognitive-complexity': ['error', 30],
      'sonarjs/no-duplicate-string': ['warn', { threshold: 5 }],

      // --- Phase 2 Items (Deferred) ---
      // 'strict-boolean-expressions': 'off', // Implicitly off, enabled later

      // Disable react/prop-types for TypeScript (type checking is handled by TypeScript)
      'react/prop-types': 'off',
    },
  },
  // 3. Prism Config - Disable import sorting (import order is critical for Prism)
  {
    files: ['src/utils/prism-config.ts'],
    rules: {
      'simple-import-sort/imports': 'off',
    },
  },
  // 3b. Storage utilities - Allow direct localStorage/sessionStorage access (it's the wrapper)
  {
    files: ['src/utils/storage.ts'],
    rules: {
      'no-restricted-globals': 'off',
      // Type parameter T is needed for caller type inference in getJSON<T>()
      '@typescript-eslint/no-unnecessary-type-parameters': 'off',
    },
  },
  // 3c. Cookie consent - Allow runtime validation for potentially corrupted storage data
  {
    files: ['src/utils/cookieConsent.ts'],
    rules: {
      // We do runtime validation for storage data which may be corrupted
      '@typescript-eslint/no-unnecessary-condition': 'off',
    },
  },
  // 4. Relaxed Rules for Test Files (No changes from previous logic)
  {
    files: [
      'e2e/**/*.{ts,tsx}',
      'tests/**/*.{ts,tsx}',
      '**/*.spec.{ts,tsx}',
      '**/*.test.{ts,tsx}',
    ],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: {
          jsx: true,
        },
      },
    },
    plugins: {
      react,
      'react-hooks': fixupPluginRules(reactHooks),
      'react-refresh': reactRefresh,
      security,
      sonarjs,
    },
    settings: {
      react: {
        version: 'detect',
      },
    },
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/restrict-template-expressions': 'off',
      '@typescript-eslint/unbound-method': 'off',
      'sonarjs/cognitive-complexity': 'off',
      'sonarjs/no-duplicate-string': 'off',
      'max-lines': 'off',
      'max-lines-per-function': 'off',
      complexity: 'off',
      '@typescript-eslint/no-empty-function': 'off',
      // Disable React hooks rules for non-React test files
      'react-hooks/rules-of-hooks': 'off',
      // Allow require() in test files for jest.isolateModules()
      '@typescript-eslint/no-require-imports': 'off',
    },
  },
  // 5. Prettier Config (Must be last)
  eslintConfigPrettier
)
