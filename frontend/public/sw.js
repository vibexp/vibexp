// Self-destruct kill switch for a legacy service worker.
//
// VibeXP no longer ships a workbox/PWA service worker. Browsers that registered
// an older one keep it until its script bytes change. Serving this minimal
// self-unregistering worker at the legacy path lets the browser's periodic
// service-worker update check replace the stale worker with this one, which then
// clears all caches, unregisters itself, and reloads open tabs with fresh
// content from the server. Harmless for clients that never had a worker here.
self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      try {
        const keys = await caches.keys()
        await Promise.all(keys.map((key) => caches.delete(key)))
      } catch {
        /* ignore */
      }
      try {
        await self.registration.unregister()
      } catch {
        /* ignore */
      }
      try {
        const clients = await self.clients.matchAll({ type: 'window' })
        for (const client of clients) {
          client.navigate(client.url)
        }
      } catch {
        /* ignore */
      }
    })()
  )
})
