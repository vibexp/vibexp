import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/postcss'

export default defineConfig({
  plugins: [react()],
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
    // No telemetry is shipped, so source maps are never uploaded anywhere —
    // don't generate them (keeps them out of the embedded single-binary image).
    sourcemap: false,
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
