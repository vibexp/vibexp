import type { ReactNode } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { SegmentedControl } from '@/components/SegmentedControl'
import { cn } from '@/lib/utils'

import { useHostedBodyView } from './readingChrome'
import type { BodyFormat, BodyViewMode } from './types'
import { useBodyViewMode } from './useBodyViewMode'

/**
 * The Raw body's surface, shared with the editor's Raw pane (#1178) so that
 * switching View → Edit changes nothing but the caret: same background, same
 * padding, same type size.
 */
export const RAW_BODY_CLASS =
  'bg-muted text-muted-foreground rounded-md p-4 font-mono text-sm'

const VIEW_OPTIONS = [
  { value: 'rendered', label: 'Rendered' },
  { value: 'raw', label: 'Raw' },
] as const

interface ResourceBodyBaseProps {
  /** What the rendered view shows (for prompts: the placeholder-rendered body). */
  content: string
  /**
   * The title the page header already shows. A leading `# Heading` in the body
   * that only repeats it is left out of the RENDERED view, so it is not read
   * twice in a row; the raw view and the content itself are untouched.
   */
  title?: string
  /** What the raw view shows. Defaults to `content` — the source, never the render. */
  rawContent?: string
  /** Only `markdown` is implemented; the prop pins the assumption for future formats. */
  format?: BodyFormat
  /** Rendered-mode-only slot above the body (the prompt's placeholder inputs). */
  renderedExtra?: ReactNode
  /**
   * Values for the `{{placeholders}}` in `content`; passing it (even empty)
   * makes the rendered view show each one as a chip or as its marked value.
   * See `MarkdownRenderer`'s prop of the same name.
   */
  placeholders?: Record<string, string>
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
 * Raw switch (drawn in the reading page's toolbar when there is one, above the
 * body otherwise), so every resource offers the source text a user is
 * about to paste into a tool. It always opens on Rendered; Raw lasts for the
 * current page view only and is stored nowhere (see `useBodyViewMode`).
 *
 * Domain-free like the rest of this package: the prompt's placeholder and
 * render machinery stays in `PromptDetail` and arrives through `renderedExtra`
 * plus a controlled `mode` (its render effects key off the mode, so it must
 * own the state).
 */
export function ResourceBody({
  content,
  title,
  rawContent,
  renderedExtra,
  placeholders,
  isLoading = false,
  loadingLabel = 'Rendering…',
  mode,
  onModeChange,
  className,
}: Readonly<ResourceBodyProps>) {
  const [ownMode, setOwnMode] = useBodyViewMode()
  const activeMode = mode ?? ownMode

  const setActiveMode = (next: BodyViewMode) => {
    if (onModeChange) onModeChange(next)
    else setOwnMode(next)
  }
  // On a reading page the switch is drawn in the page toolbar, beside Edit
  // and Copy; anywhere else the body draws it itself.
  const switchIsHosted = useHostedBodyView(activeMode, setActiveMode)

  return (
    <div className={cn('space-y-4', className)} data-testid="resource-body">
      {!switchIsHosted && (
        <div className="reading-prose-width flex justify-end">
          <SegmentedControl
            size="sm"
            aria-label="Body view"
            options={VIEW_OPTIONS}
            value={activeMode}
            onChange={value => {
              setActiveMode(value as BodyViewMode)
            }}
          />
        </div>
      )}

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
              <MarkdownRenderer
                content={content}
                hideLeadingTitle={title}
                placeholders={placeholders}
                syntaxTheme="auto"
                className="reading-body"
              />
            )}
          </>
        ) : (
          <pre
            data-testid="resource-body-raw"
            className={cn(
              'reading-body-raw overflow-x-auto whitespace-pre-wrap',
              RAW_BODY_CLASS
            )}
          >
            {rawContent ?? content}
          </pre>
        )}
      </div>
    </div>
  )
}
