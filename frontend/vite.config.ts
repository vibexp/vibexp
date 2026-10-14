import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/postcss'

/** Where the dev server sends `/config.js` when no API base URL says otherwise. */
const DEFAULT_BACKEND_ORIGIN = 'http://localhost:8080'

/**
 * The backend's origin, from the API base URL the app already talks to. A
 * relative base (the combined image's `/api/v1`) has no origin of its own, so
 * it falls back to the local backend.
 */
function backendOrigin(apiBaseUrl: string | undefined): string {
  try {
    return new URL(apiBaseUrl ?? '').origin
  } catch {
    return DEFAULT_BACKEND_ORIGIN
  }
}

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  server: {
    proxy: {
      // The backend renders /config.js: the deploy-time env and the instance's
      // site info (company name, logo, links). The combined image serves it
      // from the same origin; here the dev server forwards it, so local dev
      // reads the same values instead of Vite's index.html fallback, and saved
      // branding survives a reload. With the backend down the request fails,
      // the script does not run, and the app loads unbranded on build-time env.
      '/config.js': {
        target: backendOrigin(
          loadEnv(mode, process.cwd(), 'VITE_').VITE_API_BASE_URL
        ),
        changeOrigin: true,
      },
    },
  },
  // Build-time replacements for environment variables
  // These values are baked into the bundle during build, preventing runtime issues.
  // GTM/GA4 config moved to runtime injection (issue #57): the backend serves it
  // via /config.js and gtm.ts reads it through getEnv(), so it is no longer
  // define-d here.
  define: {
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
    // No telemetry is shipped, so source maps are never uploaded anywhere —
    // don't generate them (keeps them out of the embedded single-binary image).
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules/react-router')) return 'router'
          if (
            id.includes('node_modules/react/') ||
            id.includes('node_modules/react-dom/')
          )
            return 'vendor'
        },
      },
    },
  },
}))
