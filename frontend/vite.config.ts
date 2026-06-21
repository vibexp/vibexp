import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/postcss'
import { sentryVitePlugin } from '@sentry/vite-plugin'

export default defineConfig({
  plugins: [
    react(),
    // Sentry plugin for source map upload (only in production builds with
    // SENTRY_AUTH_TOKEN). Org/project are env-driven so the build works when
    // unset (the plugin is disabled without an auth token regardless).
    sentryVitePlugin({
      org: process.env.SENTRY_ORG,
      project: process.env.SENTRY_PROJECT,
      authToken: process.env.SENTRY_AUTH_TOKEN,
      // Only upload source maps if auth token + org/project are all provided.
      disable:
        !process.env.SENTRY_AUTH_TOKEN ||
        !process.env.SENTRY_ORG ||
        !process.env.SENTRY_PROJECT,
      // Disable telemetry to work offline
      telemetry: false,
      // Upload source maps during build
      sourcemaps: {
        assets: './dist/**',
      },
      // Release configuration
      release: {
        name:
          process.env.VITE_APP_VERSION ||
          process.env.GITHUB_SHA ||
          'development',
      },
    }),
  ],
  // Build-time replacements for environment variables
  // These values are baked into the bundle during build, preventing runtime issues
  define: {
    __VITE_GTM_ID__: JSON.stringify(process.env.VITE_GTM_ID || ''),
    __VITE_GTM_ENABLED__: process.env.VITE_GTM_ENABLED !== 'false',
    __VITE_GA4_MEASUREMENT_ID__: JSON.stringify(
      process.env.VITE_GA4_MEASUREMENT_ID || ''
    ),
    __VITE_RELEASE_SHA__: JSON.stringify(process.env.VITE_RELEASE_SHA || 'dev'),
    __VITE_RELEASE_DATE__: JSON.stringify(
      process.env.VITE_RELEASE_DATE || 'unknown'
    ),
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  css: {
    postcss: {
      plugins: [tailwindcss()],
    },
  },
  build: {
    // 'hidden' generates source maps (uploaded to Sentry via the plugin above)
    // but omits the `//# sourceMappingURL` comment, so the CDN never serves the
    // un-minified source to end users.
    sourcemap: 'hidden',
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules/react-router-dom')) return 'router'
          if (
            id.includes('node_modules/react/') ||
            id.includes('node_modules/react-dom/')
          )
            return 'vendor'
        },
      },
    },
  },
})
