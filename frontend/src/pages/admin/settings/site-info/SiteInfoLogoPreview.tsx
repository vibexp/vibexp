import { LogoMark } from '@vibexp/design-system/react'
import { useState } from 'react'

import { cn } from '@/lib/utils'

import { isHttpUrl } from './siteInfoForm'

const LOGO_HEIGHT = 36

interface SurfaceProps {
  label: string
  /** The image this surface would show, or '' for none. */
  url: string
  /** Why the VibeXP mark is shown when there is no image. */
  emptyNote: string
  className: string
  testId: string
}

function PreviewSurface({
  label,
  url,
  emptyNote,
  className,
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
      >
        {showImage ? (
          <img
            src={url}
            alt={`Logo on a ${label.toLowerCase()} background`}
            style={{ height: LOGO_HEIGHT }}
            className="max-w-full object-contain"
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
 * mark. Only a complete `http(s)` URL is requested, so a half-typed address
 * fetches nothing.
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
        className="bg-white text-zinc-900"
        testId="site-info-preview-light"
      />
      <PreviewSurface
        label="Dark theme"
        url={dark || light}
        emptyNote="No logo set, so the VibeXP mark is shown."
        className="bg-zinc-950 text-zinc-50"
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
