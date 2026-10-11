import { LogoMark } from '@vibexp/design-system/react'
import { type CSSProperties, useState } from 'react'

import { cn } from '@/lib/utils'

import { isHttpUrl } from './siteInfoForm'

const LOGO_HEIGHT = 36

// The VibeXP mark paints itself from the theme's `--foreground` and
// `--background`, so a swatch must pin both or the mark follows the app's
// current theme and vanishes on the opposite swatch. The values are the
// design system's own light and dark tokens.
const LIGHT_TOKENS = {
  '--foreground': 'oklch(0.145 0 0)',
  '--background': 'oklch(1 0 0)',
} as CSSProperties
const DARK_TOKENS = {
  '--foreground': 'oklch(0.985 0 0)',
  '--background': 'oklch(0.145 0 0)',
} as CSSProperties

interface SurfaceProps {
  label: string
  /** The image this surface would show, or '' for none. */
  url: string
  /** Why the VibeXP mark is shown when there is no image. */
  emptyNote: string
  className: string
  /** The theme tokens this surface stands for (see above). */
  tokens: CSSProperties
  testId: string
}

function PreviewSurface({
  label,
  url,
  emptyNote,
  className,
  tokens,
  testId,
}: Readonly<SurfaceProps>) {
  // Remembered per URL, so correcting the address retries the image.
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const showImage = url !== '' && failedUrl !== url

  let note = emptyNote
  if (showImage) note = 'Your logo'
  else if (url !== '')
    note = 'This image could not be loaded, so the VibeXP mark is shown.'

  return (
    <figure className="space-y-2" data-testid={testId}>
      <div
        className={cn(
          'flex h-20 items-center justify-center rounded-md border px-4',
          className
        )}
        style={tokens}
        data-testid={`${testId}-surface`}
      >
        {showImage ? (
          <img
            src={url}
            alt={`Logo on a ${label.toLowerCase()} background`}
            style={{ height: LOGO_HEIGHT }}
            // The width clamp `BrandLogo` applies, so a wide wordmark previews
            // at the size the app will show it.
            className="w-auto max-w-40 object-contain"
            onError={() => {
              setFailedUrl(url)
            }}
          />
        ) : (
          <LogoMark size={LOGO_HEIGHT} aria-hidden="true" />
        )}
      </div>
      <figcaption className="text-muted-foreground text-xs">
        <span className="text-foreground font-medium">{label}.</span> {note}
      </figcaption>
    </figure>
  )
}

/**
 * How the logo will look in the light and the dark theme, from the values
 * being typed (#1358). It follows the rule the app applies (`BrandLogo`):
 * the dark theme uses the dark-mode logo and falls back to the logo; the
 * light theme uses only the logo; anything missing or broken is the VibeXP
 * mark. Nothing is requested until the value is an `http(s)` URL; from then
 * on each keystroke requests what has been typed so far, from the admin's own
 * browser.
 */
export function SiteInfoLogoPreview({
  logoUrl,
  logoDarkUrl,
}: Readonly<{ logoUrl: string; logoDarkUrl: string }>) {
  const usable = (raw: string) => (isHttpUrl(raw.trim()) ? raw.trim() : '')
  const light = usable(logoUrl)
  const dark = usable(logoDarkUrl)

  return (
    <div
      className="grid gap-4 sm:grid-cols-2"
      aria-label="Logo preview"
      role="group"
    >
      <PreviewSurface
        label="Light theme"
        url={light}
        emptyNote="No logo set, so the VibeXP mark is shown."
        className="bg-white"
        tokens={LIGHT_TOKENS}
        testId="site-info-preview-light"
      />
      <PreviewSurface
        label="Dark theme"
        url={dark || light}
        emptyNote="No logo set, so the VibeXP mark is shown."
        className="bg-zinc-950"
        tokens={DARK_TOKENS}
        testId="site-info-preview-dark"
      />
      {dark === '' && light !== '' && (
        <p
          className="text-muted-foreground text-xs sm:col-span-2"
          data-testid="site-info-preview-dark-fallback"
        >
          No dark-mode logo is set, so the dark theme uses the logo above. Check
          that it stays readable on a dark background.
        </p>
      )}
    </div>
  )
}
