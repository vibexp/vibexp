#!/usr/bin/env node
// Generates public/firebase-messaging-sw-config.js with Firebase globals
// baked in at build time from VITE_FIREBASE_* environment variables.
//
// Fail-loud mode: set FIREBASE_CONFIG_REQUIRED=true in production build jobs
// (frontend-build-and-push.yml) to surface missing secrets as a hard build
// failure. PR test builds leave this unset so they can run without GCP
// access — the feature gracefully self-disables via isFirebaseConfigured().
//
// Note: VITE_FIREBASE_VAPID_KEY is intentionally NOT written here.
// The VAPID key is only needed by the foreground page (getToken call in
// fcm.ts) and is baked into the main bundle via import.meta.env. The SW
// only needs the six initializeApp config values.

import { writeFileSync, mkdirSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))

const vars = {
  FIREBASE_API_KEY: process.env.VITE_FIREBASE_API_KEY || '',
  FIREBASE_AUTH_DOMAIN: process.env.VITE_FIREBASE_AUTH_DOMAIN || '',
  FIREBASE_PROJECT_ID: process.env.VITE_FIREBASE_PROJECT_ID || '',
  FIREBASE_STORAGE_BUCKET: process.env.VITE_FIREBASE_STORAGE_BUCKET || '',
  FIREBASE_MESSAGING_SENDER_ID:
    process.env.VITE_FIREBASE_MESSAGING_SENDER_ID || '',
  FIREBASE_APP_ID: process.env.VITE_FIREBASE_APP_ID || '',
}

// Mirror the isFirebaseConfigured() check: apiKey + vapidKey + messagingSenderId.
// vapidKey lives in the bundle, not this file; check the two from this set.
const configured =
  Boolean(vars.FIREBASE_API_KEY) && Boolean(vars.FIREBASE_MESSAGING_SENDER_ID)

console.log(`Generating SW config: FIREBASE push configured: ${configured}`)

if (!configured && process.env.FIREBASE_CONFIG_REQUIRED === 'true') {
  console.error(
    'ERROR: VITE_FIREBASE_API_KEY and/or VITE_FIREBASE_MESSAGING_SENDER_ID are not set.\n' +
      'The Firebase service worker will not function without these values.\n' +
      'Ensure the GCP Secret Manager secrets are accessible to vibexp-ci.'
  )
  process.exit(1)
}

const lines = Object.entries(vars)
  .map(([key, value]) => `self.${key} = ${JSON.stringify(value)};`)
  .join('\n')

const content = `// Auto-generated at build time by scripts/generate-sw-config.js
// DO NOT edit manually — changes will be overwritten on the next build.
${lines}
`

const outDir = resolve(__dirname, '..', 'public')
mkdirSync(outDir, { recursive: true })

const outPath = resolve(outDir, 'firebase-messaging-sw-config.js')
writeFileSync(outPath, content, 'utf8')
console.log(`Written: ${outPath}`)
