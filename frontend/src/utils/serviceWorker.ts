/**
 * Evict orphaned / legacy service workers and their caches.
 *
 * VibeXP ships NO service worker at all. It previously registered
 * `firebase-messaging-sw.js` on demand for web push; that feature was removed
 * along with Firebase (#688). Browsers that visited an older build can still
 * carry a stale service worker — e.g. vite-plugin-pwa's `dev-sw.js` from an old
 * `npm run dev` session, the pre-rebrand "P3" production worker, or the retired
 * Firebase messaging worker. Such a worker precaches the old app and hijacks
 * this origin, serving outdated content and breaking module loads
 * ("Failed to load module script… MIME type text/html").
 *
 * This runs on every boot and unregisters every service worker it finds, then
 * deletes the caches the legacy workbox precache worker left behind. It is
 * best-effort and idempotent — a no-op once the origin is clean — and never
 * blocks startup.
 *
 * Note: a fully-hijacked tab (old worker serving a cached bundle) won't run
 * this code at all; the self-destruct workers at /sw.js and /dev-sw.js
 * (public/) recover those via the browser's periodic worker update check.
 */
const LEGACY_CACHE_PREFIXES = ['workbox-']
const LEGACY_CACHE_NAMES = new Set(['api-cache', 'avatar-cache'])

export function cleanupLegacyServiceWorkers(): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
    return
  }

  navigator.serviceWorker
    .getRegistrations()
    .then(registrations => {
      for (const registration of registrations) {
        void registration.unregister()
      }
    })
    .catch(() => {
      /* best-effort; never block app startup */
    })

  if ('caches' in window) {
    caches
      .keys()
      .then(keys => {
        for (const key of keys) {
          const isLegacy =
            LEGACY_CACHE_NAMES.has(key) ||
            LEGACY_CACHE_PREFIXES.some(prefix => key.startsWith(prefix))
          if (isLegacy) {
            void caches.delete(key)
          }
        }
      })
      .catch(() => {
        /* best-effort */
      })
  }
}
