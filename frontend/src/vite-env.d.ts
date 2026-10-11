/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly DEV: boolean
  readonly PROD: boolean
  readonly MODE: string
  readonly BASE_URL: string
  readonly SSR: boolean
  readonly VITE_GTM_ID: string
  readonly VITE_GA4_MEASUREMENT_ID: string
  readonly VITE_API_BASE_URL: string
  readonly VITE_MCP_ENDPOINT: string
  readonly VITE_ERROR_TYPE_BASE_URI: string
  readonly VITE_RELEASE_SHA: string
  readonly VITE_RELEASE_DATE: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
