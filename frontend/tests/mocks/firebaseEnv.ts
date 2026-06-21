/**
 * Jest mock for src/lib/firebaseEnv.ts.
 * Returns empty/false values by default so tests that don't need Firebase
 * see it as unconfigured. Override with jest.spyOn or jest.mock factory.
 */

export function getFirebaseConfig() {
  return {
    apiKey: '',
    authDomain: '',
    projectId: '',
    storageBucket: '',
    messagingSenderId: '',
    appId: '',
  }
}

export function getFirebaseVapidKey(): string {
  return ''
}

export function isFirebaseConfigured(): boolean {
  return false
}
