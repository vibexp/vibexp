/**
 * Instance site info: the company branding an instance admin configures
 * (epic #1352), as the frontend consumes it.
 *
 * The backend exposes the same eight values on two public surfaces: inlined in
 * `/config.js` as `window.__VIBEXP_SITE_INFO__` (read synchronously, so the
 * first paint is already branded) and `GET /api/v1/site-info` (refetched after
 * an admin saves). This module is the pure part: the shape, reading the
 * inlined global, and the fallback rules every surface shares. The reactive
 * part is `SiteInfoProvider` / `useSiteInfo`.
 */

/** The product's own name: shown when no company name is configured. */
export const PRODUCT_NAME = 'VibeXP'

/**
 * How every browser tab title ends: "{company name} · VibeXP", or "VibeXP"
 * when no company name is configured (#1360). The product is always named —
 * a company name never replaces it — and a company that is itself called
 * VibeXP is not named twice.
 */
export function siteTitleSuffix(companyName: string | null): string {
  const company = companyName?.trim() ?? ''
  if (company === '' || company.toLowerCase() === PRODUCT_NAME.toLowerCase()) {
    return PRODUCT_NAME
  }
  return `${company} · ${PRODUCT_NAME}`
}

/** Where "Powered by VibeXP" and the VibeXP mark link. */
export const PRODUCT_URL = 'https://vibexp.io'

/** The eight site info values. `null` means "not configured". */
export interface SiteInfo {
  company_name: string | null
  company_email: string | null
  website: string | null
  support_url: string | null
  terms_url: string | null
  privacy_url: string | null
  logo_url: string | null
  logo_dark_url: string | null
}

/** Site info plus the values derived from it by the shared fallback rules. */
export interface DerivedSiteInfo extends SiteInfo {
  /** The company name, or the product name when none is configured. */
  displayName: string
  /**
   * Whether the instance shows a company's branding (a name or either logo),
   * in which case the product is credited as "Powered by VibeXP". A dark logo
   * alone counts: `BrandLogo` shows it in dark mode.
   */
  hasCompanyBranding: boolean
  /** `support_url`, else `mailto:company_email`, else `null` (no link). */
  supportHref: string | null
}

export const EMPTY_SITE_INFO: SiteInfo = {
  company_name: null,
  company_email: null,
  website: null,
  support_url: null,
  terms_url: null,
  privacy_url: null,
  logo_url: null,
  logo_dark_url: null,
}

declare global {
  interface Window {
    /** Site info injected by the backend's `/config.js` (#1355). */
    __VIBEXP_SITE_INFO__?: unknown
  }
}

/** A non-empty trimmed string, or null for anything else. */
function text(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed === '' ? null : trimmed
}

/** `text`, further limited to an absolute http(s) URL. */
function httpUrl(value: unknown): string | null {
  const candidate = text(value)
  if (candidate === null) return null
  try {
    const url = new URL(candidate)
    return url.protocol === 'http:' || url.protocol === 'https:'
      ? candidate
      : null
  } catch {
    return null
  }
}

/**
 * Normalises an untrusted value into `SiteInfo`.
 *
 * The server validates what it stores, so this is a second line: whatever
 * arrives, a value that is not a string becomes null and a URL that is not
 * http(s) becomes null, so nothing unexpected can reach an `href` or `src`.
 * Anything that is not an object at all is "nothing configured".
 */
export function toSiteInfo(raw: unknown): SiteInfo {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    return { ...EMPTY_SITE_INFO }
  }
  const source = new Map<string, unknown>(Object.entries(raw))
  return {
    company_name: text(source.get('company_name')),
    company_email: text(source.get('company_email')),
    website: httpUrl(source.get('website')),
    support_url: httpUrl(source.get('support_url')),
    terms_url: httpUrl(source.get('terms_url')),
    privacy_url: httpUrl(source.get('privacy_url')),
    logo_url: httpUrl(source.get('logo_url')),
    logo_dark_url: httpUrl(source.get('logo_dark_url')),
  }
}

/**
 * Reads the site info `/config.js` inlined. Absent (`/config.js` could not be
 * loaded, or a non-browser environment) is "nothing configured".
 */
export function readInlinedSiteInfo(): SiteInfo {
  if (typeof window === 'undefined') return { ...EMPTY_SITE_INFO }
  return toSiteInfo(window.__VIBEXP_SITE_INFO__)
}

/** Applies the fallback rules every branded surface shares. */
export function deriveSiteInfo(info: SiteInfo): DerivedSiteInfo {
  let supportHref: string | null = null
  if (info.support_url !== null) {
    supportHref = info.support_url
  } else if (info.company_email !== null) {
    supportHref = `mailto:${info.company_email}`
  }
  return {
    ...info,
    displayName: info.company_name ?? PRODUCT_NAME,
    hasCompanyBranding:
      info.company_name !== null ||
      info.logo_url !== null ||
      info.logo_dark_url !== null,
    supportHref,
  }
}

/**
 * Fired on `window` after an instance admin saves the site info, so the
 * provider refetches and every open surface rebrands without a reload.
 */
export const SITE_INFO_SAVED_EVENT = 'vibexp:site-info-saved'

/** Tells the app the site info changed. Call it after a successful save. */
export function notifySiteInfoSaved(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(SITE_INFO_SAVED_EVENT))
}
