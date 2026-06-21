/**
 * Firebase environment configuration helper.
 *
 * Reads Firebase-related Vite environment variables and exposes them as typed
 * functions so that tests can mock this module rather than dealing with
 * import.meta.env directly (which is not available in Jest/CommonJS).
 */

export function getFirebaseConfig() {
  return {
    apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
    authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
    projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
    storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
    messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
    appId: import.meta.env.VITE_FIREBASE_APP_ID,
  }
}

export function getFirebaseVapidKey(): string {
  return import.meta.env.VITE_FIREBASE_VAPID_KEY
}

export function isFirebaseConfigured(): boolean {
  return Boolean(
    import.meta.env.VITE_FIREBASE_API_KEY &&
    import.meta.env.VITE_FIREBASE_VAPID_KEY &&
    import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID
  )
}
