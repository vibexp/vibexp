// Global type declarations for gtag and dataLayer
// These are always defined in index.html before any other scripts load
declare global {
  interface Window {
    dataLayer: Record<string, unknown>[]
    gtag: (...args: unknown[]) => void
  }
}

import { STORAGE_KEYS } from '../constants/storageKeys'
import { storage } from './storage'

const CONSENT_EXPIRY_DAYS = 7 // Re-show banner after 7 days for declined consent

interface ConsentData {
  status: 'granted' | 'denied'
  timestamp: number
}

/**
 * Grant cookie consent and update Google Consent Mode v2
 * This should be called when users explicitly grant consent (e.g., by accepting the banner)
 * or implicitly grant consent (e.g., by logging in and agreeing to privacy policy)
 *
 * Granted consent is stored indefinitely without expiry.
 */
export function grantCookieConsent(): void {
  // Save consent to localStorage with timestamp
  const consentData: ConsentData = {
    status: 'granted',
    timestamp: Date.now(),
  }
  storage.set(STORAGE_KEYS.COOKIE_CONSENT, consentData)

  // Update Google Consent Mode v2 (Unlocks GA4/Google Ads)
  // gtag is defined in index.html before any other scripts load
  window.gtag('consent', 'update', {
    ad_storage: 'granted',
    ad_user_data: 'granted',
    ad_personalization: 'granted',
    analytics_storage: 'granted',
  })

  // Push Custom Event (Triggers 3rd party tags in GTM)
  window.dataLayer.push({
    event: 'cookie_consent_update',
    consent_status: 'granted',
  })
}

/**
 * Deny cookie consent
 * This should be called when users explicitly decline cookies
 *
 * Declined consent expires after 7 days, after which the banner will be shown again.
 */
export function denyCookieConsent(): void {
  // Save decline to localStorage with timestamp
  const consentData: ConsentData = {
    status: 'denied',
    timestamp: Date.now(),
  }
  storage.set(STORAGE_KEYS.COOKIE_CONSENT, consentData)

  // Explicitly set consent to denied (reinforces the default state)
  // This ensures Google knows the user explicitly declined
  window.gtag('consent', 'update', {
    ad_storage: 'denied',
    ad_user_data: 'denied',
    ad_personalization: 'denied',
    analytics_storage: 'denied',
  })

  // Push decline event to dataLayer
  window.dataLayer.push({
    event: 'cookie_consent_update',
    consent_status: 'denied',
  })
}

/**
 * Check if user has already granted cookie consent
 * @returns true if user has granted consent, false otherwise
 */
export function hasGrantedCookieConsent(): boolean {
  // First try to parse as ConsentData JSON
  const consentData = storage.getJSON<ConsentData>(STORAGE_KEYS.COOKIE_CONSENT)
  if (consentData?.status === 'granted') {
    return true
  }

  // Handle legacy plain string format
  const legacyValue = storage.get(STORAGE_KEYS.COOKIE_CONSENT)
  return legacyValue === 'granted'
}

/**
 * Check if user has made a consent decision that is still valid
 * - Granted consent: valid indefinitely
 * - Declined consent: expires after 7 days
 *
 * @returns true if user has a valid consent decision, false otherwise
 */
export function hasCookieConsentDecision(): boolean {
  // First try to parse as ConsentData JSON
  const consentData = storage.getJSON<ConsentData>(STORAGE_KEYS.COOKIE_CONSENT)

  if (consentData) {
    // Runtime validation: ensure status is a valid value (storage data could be corrupted)
    if (consentData.status !== 'granted' && consentData.status !== 'denied') {
      return false
    }

    // If consent was granted, it's valid indefinitely
    if (consentData.status === 'granted') {
      return true
    }

    // If consent was denied, check if it has expired (7 days)
    const daysSinceDecision =
      (Date.now() - consentData.timestamp) / (1000 * 60 * 60 * 24)
    return daysSinceDecision < CONSENT_EXPIRY_DAYS
  }

  // Handle legacy plain string format
  const legacyValue = storage.get(STORAGE_KEYS.COOKIE_CONSENT)
  return legacyValue === 'granted' || legacyValue === 'denied'
}
