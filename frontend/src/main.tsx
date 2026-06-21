import './styles/index.css'

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import App from './App.tsx'
import { GTM_ENABLED, initializeGTM } from './utils/gtm'
import { initSentry } from './utils/sentry'
import { cleanupLegacyServiceWorkers } from './utils/serviceWorker'
import { storageUtils } from './utils/storage'

// Initialize Sentry for error tracking (must be first)
initSentry()

// One-time cleanup: remove any legacy JWT auth tokens from localStorage.
// Returning users may still have `auth_token` or `vx_auth_token` from the
// pre-WorkOS auth flow. The app now uses httpOnly session cookies exclusively,
// so any local token is invalid — migrateStorageKeys() wipes it on init.
storageUtils.migrateStorageKeys()

// Evict stale/legacy service workers (e.g. an old vite-plugin-pwa dev worker)
// that can hijack this origin and serve outdated content. No-op once clean.
cleanupLegacyServiceWorkers()

// Initialize Google Tag Manager only if explicitly enabled
if (GTM_ENABLED) {
  initializeGTM()
}

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Failed to find the root element')
}
createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>
)
