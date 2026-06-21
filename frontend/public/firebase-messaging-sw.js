importScripts('/firebase-messaging-sw-config.js')
importScripts(
  'https://www.gstatic.com/firebasejs/12.13.0/firebase-app-compat.js'
)
importScripts(
  'https://www.gstatic.com/firebasejs/12.13.0/firebase-messaging-compat.js'
)

firebase.initializeApp({
  apiKey: self.FIREBASE_API_KEY || '',
  authDomain: self.FIREBASE_AUTH_DOMAIN || '',
  projectId: self.FIREBASE_PROJECT_ID || '',
  storageBucket: self.FIREBASE_STORAGE_BUCKET || '',
  messagingSenderId: self.FIREBASE_MESSAGING_SENDER_ID || '',
  appId: self.FIREBASE_APP_ID || '',
})

const messaging = firebase.messaging()

messaging.onBackgroundMessage(payload => {
  const { title, body } = payload.notification || {}
  const actionUrl = payload.data?.action_url || '/'
  self.registration.showNotification(title || 'VibeXP', {
    body: body || '',
    icon: '/favicon.svg',
    data: { url: actionUrl },
  })
})

self.addEventListener('notificationclick', event => {
  event.notification.close()
  const rawUrl = event.notification.data?.url || '/'
  // Only allow same-origin relative paths to prevent open-redirect via crafted push payloads
  const url =
    typeof rawUrl === 'string' && rawUrl.startsWith('/') ? rawUrl : '/'
  event.waitUntil(
    clients
      .matchAll({ type: 'window', includeUncontrolled: true })
      .then(clientList => {
        for (const client of clientList) {
          try {
            const clientPath = new URL(client.url).pathname
            if (clientPath.startsWith(url) && 'focus' in client)
              return client.focus()
          } catch {
            // ignore invalid client URLs
          }
        }
        if (clients.openWindow) return clients.openWindow(url)
      })
  )
})
