import type { ReactNode } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { SegmentedControl } from '@/components/SegmentedControl'
import { cn } from '@/lib/utils'

import type { BodyFormat, BodyViewMode } from './types'
import { useBodyViewMode } from './useBodyViewMode'

const VIEW_OPTIONS = [
  { value: 'rendered', label: 'Rendered' },
  { value: 'raw', label: 'Raw' },
] as const

interface ResourceBodyBaseProps {
  /** What the rendered view shows (for prompts: the placeholder-rendered body). */
  content: string
  /** What the raw view shows. Defaults to `content` — the source, never the render. */
  rawContent?: string
  /** Only `markdown` is implemented; the prop pins the assumption for future formats. */
  format?: BodyFormat
  /** Rendered-mode-only slot above the body (the prompt's placeholder inputs). */
  renderedExtra?: ReactNode
  /** Replaces the rendered body with a spinner while it is being produced. */
  isLoading?: boolean
  loadingLabel?: string
  className?: string
}

/**
 * Controlled and uncontrolled are the only two legal shapes. Split as a union
 * rather than two independent optionals because `mode` without `onModeChange`
 * would type-check and then be silently inert — the switch would render and
 * respond to clicks while the caller's mode never moved.
 */
type ResourceBodyModeProps =
  | { mode: BodyViewMode; onModeChange: (mode: BodyViewMode) => void }
  | { mode?: undefined; onModeChange?: undefined }

export type ResourceBodyProps = ResourceBodyBaseProps & ResourceBodyModeProps

/**
 * The one body treatment every resource detail page uses (#901).
 *
 * A bare article — no card chrome (epic #899 decision A) — with a Rendered /
 * Raw switch above it, so every resource offers the source text a user is
 * about to paste into a tool. The chosen mode is remembered per browser under
 * a single shared key, following the details-column collapse precedent.
 *
 * Domain-free like the rest of this package: the prompt's placeholder and
 * render machinery stays in `PromptDetail` and arrives through `renderedExtra`
 * plus a controlled `mode` (its render effects key off the mode, so it must
 * own the state).
 */
export function ResourceBody({
  content,
  rawContent,
  renderedExtra,
  isLoading = false,
  loadingLabel = 'Rendering…',
  mode,
  onModeChange,
  className,
}: Readonly<ResourceBodyProps>) {
  const [storedMode, setStoredMode] = useBodyViewMode()
  const activeMode = mode ?? storedMode

  const handleChange = (value: string) => {
    const next = value as BodyViewMode
    if (onModeChange) onModeChange(next)
    else setStoredMode(next)
  }

  return (
    <div className={cn('space-y-4', className)} data-testid="resource-body">
      <div className="flex justify-end">
        <SegmentedControl
          size="sm"
          aria-label="Body view"
          options={VIEW_OPTIONS}
          value={activeMode}
          onChange={handleChange}
        />
      </div>

      {/*
        Only the active view is mounted, and it carries `role="tabpanel"` to
        pair with the switch's `role="tab"` — both the ARIA contract and what
        the e2e suite reads (`getByRole('tabpanel')` is strict, so a second
        mounted panel would break it, as would rendering the body twice).
      */}
      <div role="tabpanel" aria-label="Resource body" className="space-y-4">
        {activeMode === 'rendered' ? (
          <>
            {renderedExtra}
            {isLoading ? (
              <LoadingSpinner label={loadingLabel} />
            ) : (
              <MarkdownRenderer content={content} syntaxTheme="auto" />
            )}
          </>
        ) : (
          <pre
            data-testid="resource-body-raw"
            className="bg-muted text-muted-foreground overflow-x-auto rounded-md p-4 font-mono text-xs whitespace-pre-wrap"
          >
            {rawContent ?? content}
          </pre>
        )}
      </div>
    </div>
  )
}
