/**
 * The app header's height in pixels, read from the design-system token
 * `--header-height` (3.5rem). CSS uses the token directly (`top-header`,
 * `calc(… var(--header-height))`); the two IntersectionObserver margins on a
 * reading page need it as a number, and read it here so a change to the token
 * moves them too.
 */
const FALLBACK_HEADER_HEIGHT_PX = 56

export function headerHeightPx(): number {
  const root = document.documentElement
  const styles = getComputedStyle(root)
  const rem = Number.parseFloat(styles.getPropertyValue('--header-height'))
  const rootFontSize = Number.parseFloat(styles.fontSize)
  // No stylesheet (a unit test) or a value that is not in rem: the token's
  // documented size.
  if (!Number.isFinite(rem) || !Number.isFinite(rootFontSize)) {
    return FALLBACK_HEADER_HEIGHT_PX
  }
  return rem * rootFontSize
}

/**
 * The sticky title bar under the header, in pixels (4rem at the default root
 * size). `scroll-margin-top` in `styles/index.css` adds the same 4rem to
 * `--header-height`, so an anchor lands just below both.
 */
export const TITLE_BAR_HEIGHT_PX = 64
