/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly DEV: boolean
  readonly PROD: boolean
  readonly MODE: string
  readonly BASE_URL: string
  readonly SSR: boolean
  readonly VITE_GTM_ID: string
  readonly VITE_GTM_ENABLED: string
  readonly VITE_GA4_MEASUREMENT_ID: string
  readonly VITE_API_BASE_URL: string
  readonly VITE_SENTRY_DSN: string
  readonly VITE_SITE_NAME: string
  readonly VITE_SITE_LEGAL_NAME: string
  readonly VITE_SITE_URL: string
  readonly VITE_TERMS_URL: string
  readonly VITE_PRIVACY_URL: string
  readonly VITE_SUPPORT_EMAIL: string
  readonly VITE_BRAND_LOGO_URL: string
  readonly VITE_MCP_ENDPOINT: string
  readonly VITE_ERROR_TYPE_BASE_URI: string
  readonly VITE_RELEASE_SHA: string
  readonly VITE_RELEASE_DATE: string
  readonly VITE_FIREBASE_API_KEY: string
  readonly VITE_FIREBASE_AUTH_DOMAIN: string
  readonly VITE_FIREBASE_PROJECT_ID: string
  readonly VITE_FIREBASE_STORAGE_BUCKET: string
  readonly VITE_FIREBASE_MESSAGING_SENDER_ID: string
  readonly VITE_FIREBASE_APP_ID: string
  readonly VITE_FIREBASE_VAPID_KEY: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
