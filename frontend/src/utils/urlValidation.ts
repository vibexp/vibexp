/**
 * Safely redirects to a URL after validation against a caller-supplied
 * allowlist of hosts. Throws if the URL is not from an allowed domain.
 * Prevents open redirect vulnerabilities.
 */
export function safeRedirect(
  url: string,
  allowedDomains: readonly string[]
): void {
  try {
    const parsedUrl = new URL(url)

    // Pin the scheme to https. All legitimate redirect targets (Stripe, GitHub)
    // are HTTPS; this also rejects javascript:/data:/blob: schemes outright as
    // defense-in-depth, independent of the hostname allowlist.
    if (parsedUrl.protocol !== 'https:') {
      throw new Error(
        `Redirect blocked: only https URLs are allowed, got "${parsedUrl.protocol}"`
      )
    }

    // Reject URLs with credentials (prevents phishing: https://stripe.com@evil.com)
    if (parsedUrl.username || parsedUrl.password) {
      throw new Error(
        'Redirect blocked: URLs with embedded credentials are not allowed'
      )
    }

    const isAllowed = allowedDomains.some(
      domain =>
        parsedUrl.hostname === domain ||
        parsedUrl.hostname.endsWith(`.${domain}`)
    )

    if (!isAllowed) {
      throw new Error(
        `Redirect blocked: URL ${url} is not from an allowed domain`
      )
    }

    window.location.href = url
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error(`Invalid URL format: ${url}`)
    }
    throw error
  }
}
