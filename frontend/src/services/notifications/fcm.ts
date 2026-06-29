import { getApps, initializeApp } from 'firebase/app'
import {
  deleteToken,
  getMessaging,
  getToken,
  type Messaging,
  onMessage,
} from 'firebase/messaging'

import { apiClient } from '@/lib/apiClient'
import {
  getFirebaseConfig,
  getFirebaseVapidKey,
  isFirebaseConfigured,
} from '@/lib/firebaseEnv'
import { toast } from '@/lib/toast'

class FCMService {
  private messagingInstance: Messaging | null = null

  private getMessagingInstance(): Messaging | null {
    try {
      const app =
        getApps().length === 0
          ? initializeApp(getFirebaseConfig())
          : getApps()[0]
      return getMessaging(app)
    } catch {
      return null
    }
  }

  /**
   * Requests browser notification permission and registers an FCM device token.
   * Returns true if permission was granted and registration succeeded.
   * Must only be called on explicit user action — never on page load.
   */
  async requestPermissionAndRegister(): Promise<boolean> {
    const permission = await Notification.requestPermission()
    if (permission !== 'granted') {
      return false
    }

    this.messagingInstance = this.getMessagingInstance()
    if (!this.messagingInstance) return false

    try {
      // register() resolves once the SW is registered, but getToken() triggers
      // PushManager.subscribe(), which throws "AbortError: no active Service
      // Worker" until the SW is actually active. Await ready to get the active
      // registration and eliminate that race.
      await navigator.serviceWorker.register('/firebase-messaging-sw.js')
      const registration = await navigator.serviceWorker.ready

      const token = await getToken(this.messagingInstance, {
        vapidKey: getFirebaseVapidKey(),
        serviceWorkerRegistration: registration,
      })

      if (!token) return false

      await apiClient.post<unknown>('/device-tokens', {
        token,
        platform: 'web',
        user_agent: navigator.userAgent,
      })

      // Set up foreground message handler
      onMessage(this.messagingInstance, payload => {
        const title = payload.notification?.title ?? 'VibeXP'
        const body = payload.notification?.body ?? ''
        toast.info(`${title}: ${body}`)
      })

      return true
    } catch (error) {
      console.error(
        '[FCM] requestPermissionAndRegister failed:',
        error instanceof Error ? error : new Error(String(error))
      )
      return false
    }
  }

  /**
   * Revokes the current FCM token and removes the device token from the backend.
   * Best-effort — ignores errors to ensure clean local state regardless of backend state.
   */
  async revokeToken(): Promise<void> {
    this.messagingInstance =
      this.messagingInstance ?? this.getMessagingInstance()
    if (!this.messagingInstance) return

    try {
      const registration = await navigator.serviceWorker.getRegistration(
        '/firebase-messaging-sw.js'
      )
      if (!registration) return

      const token = await getToken(this.messagingInstance, {
        vapidKey: getFirebaseVapidKey(),
        serviceWorkerRegistration: registration,
      })
      if (!token) return

      await deleteToken(this.messagingInstance)
      await apiClient.delete<unknown>('/device-tokens', { token })
    } catch {
      // Best-effort cleanup — do not propagate errors
    }
  }

  /**
   * Returns true if all required Firebase environment variables are set.
   * When false, browser push notifications are unavailable and the UI should
   * show a disabled/unconfigured state.
   */
  isFCMConfigured(): boolean {
    return isFirebaseConfigured()
  }
}

export const fcmService = new FCMService()
