/**
 * Evict orphaned / legacy service workers and their caches.
 *
 * VibeXP no longer ships a PWA / workbox service worker. The only legitimate
 * service worker is `firebase-messaging-sw.js`, registered on demand for push
 * notifications (see services/notifications/fcm.ts). However, browsers that
 * visited an older build can still carry a stale service worker — e.g.
 * vite-plugin-pwa's `dev-sw.js` from an old `npm run dev` session, or the
 * pre-rebrand "P3" production worker. Such a worker precaches the old app and
 * hijacks this origin, serving outdated content and breaking module loads
 * ("Failed to load module script… MIME type text/html").
 *
 * This runs on every boot and unregisters any service worker that is NOT the
 * Firebase messaging worker, then deletes the caches the legacy workbox
 * precache worker left behind. It is best-effort and idempotent — a no-op once
 * the origin is clean — and never blocks startup.
 *
 * Note: a fully-hijacked tab (old worker serving a cached bundle) won't run
 * this code at all; the self-destruct workers at /sw.js and /dev-sw.js
 * (public/) recover those via the browser's periodic worker update check.
 */
const KEEP_WORKER = '/firebase-messaging-sw.js'
const LEGACY_CACHE_PREFIXES = ['workbox-']
const LEGACY_CACHE_NAMES = new Set(['api-cache', 'avatar-cache'])

export function cleanupLegacyServiceWorkers(): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
    return
  }

  navigator.serviceWorker
    .getRegistrations()
    .then((registrations) => {
      for (const registration of registrations) {
        const scriptURL =
          registration.active?.scriptURL ??
          registration.waiting?.scriptURL ??
          registration.installing?.scriptURL ??
          ''
        if (!scriptURL.endsWith(KEEP_WORKER)) {
          void registration.unregister()
        }
      }
    })
    .catch(() => {
      /* best-effort; never block app startup */
    })

  if ('caches' in window) {
    caches
      .keys()
      .then((keys) => {
        for (const key of keys) {
          const isLegacy =
            LEGACY_CACHE_NAMES.has(key) ||
            LEGACY_CACHE_PREFIXES.some((prefix) => key.startsWith(prefix))
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
