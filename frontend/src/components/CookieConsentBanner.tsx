import { useEffect, useState } from 'react'

import {
  denyCookieConsent,
  grantCookieConsent,
  hasCookieConsentDecision,
} from '@/utils/cookieConsent'

/**
 * CookieConsentBanner - A GDPR/CCPA compliant cookie consent banner
 *
 * This component is displayed on the SignInPage for users who haven't made a consent decision.
 * Once users log in, consent is automatically granted since they agreed to the privacy policy.
 *
 * Features:
 * - Shows on first visit for users who haven't consented
 * - Respects user's previous consent choice (localStorage)
 * - Updates Google Consent Mode v2 when users accept cookies
 * - Triggers GTM tags via dataLayer event
 * - Responsive design for mobile and desktop
 * - Accessible with proper ARIA labels
 */
export function CookieConsentBanner() {
  const [showBanner, setShowBanner] = useState(false)

  useEffect(() => {
    // Check if user has already made a consent decision
    if (!hasCookieConsentDecision()) {
      setShowBanner(true)
    }
  }, [])

  const handleAcceptCookies = () => {
    grantCookieConsent()
    setShowBanner(false)
  }

  const handleDeclineCookies = () => {
    denyCookieConsent()
    setShowBanner(false)
  }

  if (!showBanner) return null

  return (
    <div
      className="fixed bottom-0 left-0 right-0 z-50 bg-primary text-primary-foreground p-4 shadow-lg"
      role="dialog"
      aria-labelledby="cookie-consent-title"
      aria-describedby="cookie-consent-description"
    >
      <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div className="flex-1">
          <p id="cookie-consent-title" className="font-semibold text-base mb-1">
            We use cookies
          </p>
          <p
            id="cookie-consent-description"
            className="text-sm text-primary-foreground/70 leading-relaxed"
          >
            We use cookies to improve your experience and analyze site usage. By
            clicking <span className="font-semibold">Accept</span>, you agree to
            our use of cookies.
          </p>
        </div>
        <div className="flex flex-col sm:flex-row gap-2 sm:ml-4 w-full sm:w-auto">
          <button
            onClick={handleDeclineCookies}
            className="px-4 py-2.5 text-sm font-medium text-primary-foreground border border-primary-foreground/30 rounded-md hover:bg-primary-foreground/10 transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-primary"
          >
            Decline
          </button>
          <button
            onClick={handleAcceptCookies}
            className="px-4 py-2.5 text-sm font-semibold text-primary bg-primary-foreground rounded-md hover:bg-primary-foreground/90 transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:ring-offset-primary"
          >
            Accept
          </button>
        </div>
      </div>
    </div>
  )
}
