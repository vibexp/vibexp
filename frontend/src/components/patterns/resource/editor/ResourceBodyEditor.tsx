import { AlertCircle, Download, Wand2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { forwardRef, useState } from 'react'

import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import type { BodyFormat } from '@/components/patterns/reading-page'
import {
  RAW_BODY_CLASS,
  useBodyViewMode,
} from '@/components/patterns/reading-page'
import { PromptMentionTextarea } from '@/components/PromptMentionTextarea'
import type { SegmentedOption } from '@/components/SegmentedControl'
import { SegmentedControl } from '@/components/SegmentedControl'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

/**
 * The one minimum height every body pane shares.
 *
 * A minimum is not the thing #914 removes — four different magic numbers are
 * (22 rows, 24 rows, 30 rows, and a hard 600px floor on the prompt). One
 * exported constant on the Tailwind scale is what makes "the editor is the
 * same size everywhere" a fact rather than a coincidence, and it is what the
 * tests assert against, so the two cannot drift.
 */
export const BODY_EDITOR_MIN_HEIGHT = 'min-h-96'

/**
 * The floor in lines, for the browsers where it bites before the pixel one.
 * Deliberately below {@link BODY_EDITOR_MIN_HEIGHT} at the editor's type
 * scale, so the pixel minimum is what actually governs and both panes — Write
 * and Preview — start exactly the same height.
 */
export const BODY_EDITOR_MIN_ROWS = 12

/**
 * Auto-grow, as one CSS declaration.
 *
 * `field-sizing: content` makes the textarea's intrinsic height track what is
 * typed into it, which is the whole point of the issue: a long body pushes the
 * page down instead of scrolling inside a small box. One CSS declaration
 * rather than a scroll-height effect, so it applies identically to both write
 * panes; where the property is unsupported the pane degrades to the shared
 * minimum, not to the fixed box it replaces.
 *
 * `resize-y` is UNCONDITIONAL, not a fallback — both textareas already had it
 * and dragging the handle is a habit worth keeping. The trade is real: a drag
 * writes an inline `height`, which outranks `field-sizing` and pins that one
 * element for the rest of the session. Deliberate: the user asked for a size.
 */
/*
 * The Raw pane IS the reading page's Raw block with a caret (#1178): the same
 * surface (`RAW_BODY_CLASS`), no border, and `md:text-sm` so the plain
 * `Textarea`'s own `text-base md:text-sm` cannot win back the type size at
 * any breakpoint. The error is a ring, since there is no border left to colour.
 */
const WRITE_TEXTAREA_CLASS = `${RAW_BODY_CLASS} ${BODY_EDITOR_MIN_HEIGHT} field-sizing-content resize-y border-0 md:text-sm`

/** Which pane of the editor is showing. */
export type BodyEditorView = 'write' | 'preview' | 'render'

/** Swaps the plain textarea for the prompt `@`-mention one. */
export interface BodyEditorMentionsExtension {
  /** Prompt slug to keep out of the picker — the one being edited. */
  excludeCurrentPrompt?: string
}

/** Adds a third option beside Rendered and Raw. */
export interface BodyEditorRenderExtension {
  /** The panel the tab shows. The editor renders it and knows nothing else. */
  content: ReactNode
  /** Disables the option while whatever the panel needs is still loading. */
  disabled?: boolean
}

/**
 * The prompt-only behaviour, as three independently optional opt-ins.
 *
 * Everything here used to be unconditional in `pages/prompts/editor`, which is
 * why no other resource could reuse the tab shell around it. A descriptor
 * passes what its kind actually has; a kind that passes nothing gets Rendered
 * and Raw and no prompt vocabulary anywhere in its markup.
 */
export interface ResourceBodyEditorExtensions {
  mentions?: BodyEditorMentionsExtension
  render?: BodyEditorRenderExtension
  /** Called by a "Load template" button rendered beside the switch. */
  templates?: () => void
}

interface ResourceBodyEditorBaseProps {
  value: string
  onChange: (next: string) => void
  /** Only `markdown` is implemented; the prop pins the assumption, as on the reading side. */
  format?: BodyFormat
  extensions?: ResourceBodyEditorExtensions
  placeholder?: string
  /** Inline validation message, rendered once under the Raw pane. */
  error?: string
  /** react-hook-form registers the leaf through this, so `touched` works. */
  onBlur?: () => void
  disabled?: boolean
  className?: string
  /** Accessible name for the textarea when no visible `<label>` points at it. */
  'aria-label'?: string
  'data-testid'?: string
  /**
   * The ids shadcn's `FormControl` (a Radix `Slot`) clones onto its child.
   * They are forwarded to the `<textarea>` leaf — see the note on
   * {@link ResourceBodyEditor} about the `mentions` extension.
   */
  id?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}

/**
 * Controlled and uncontrolled are the only two legal shapes — the same union,
 * for the same reason, as `ResourceBody`'s view mode: a `view` without an
 * `onViewChange` would type-check and then be silently inert.
 */
type ResourceBodyEditorViewProps =
  | { view: BodyEditorView; onViewChange: (view: BodyEditorView) => void }
  | { view?: undefined; onViewChange?: undefined }

export type ResourceBodyEditorProps = ResourceBodyEditorBaseProps &
  ResourceBodyEditorViewProps

/**
 * The one markdown body editor every resource form uses (#914).
 *
 * The reading page's body treatment with a caret (#1178): the same
 * right-aligned Rendered | Raw switch `ResourceBody` shows, no card chrome, a
 * Raw pane that is the Raw block turned into a textarea that grows with its
 * content, and a Rendered pane that is exactly what `ResourceBody` renders —
 * the very same {@link MarkdownRenderer}, `reading-body` class included — so
 * switching View → Edit moves nothing.
 *
 * The public view names predate that and stay (`write` is Raw, `preview` is
 * Rendered) so a controlled caller is unaffected. Edit always opens on Raw;
 * choosing Raw or Rendered here writes the shared reading preference, so the
 * next reading page opens the way the user last looked at the body.
 *
 * It is the editing counterpart of `ResourceBody` (#901) and, like it, is
 * domain-free: the prompt's `@` mentions, its Render tab and its template
 * loader arrive through {@link ResourceBodyEditorExtensions} rather than being
 * baked in, which is what lets artifacts, blueprints and memories — which had
 * a plain fixed-height textarea and no preview at all — use the same component.
 *
 * **Both Raw panes are the same leaf.** `disabled`, the accessible name,
 * the `FormControl` slot ids and the forwarded ref reach the `<textarea>`
 * whichever pane renders — a component in between that swallowed them would
 * leave a label dangling or a control editable mid-save, and it would do it
 * silently. That is why `PromptMentionTextarea` forwards a ref and accepts the
 * form-control props rather than the `mentions` extension being fenced off
 * from generated forms.
 */
export const ResourceBodyEditor = forwardRef<
  HTMLTextAreaElement,
  Readonly<ResourceBodyEditorProps>
>(function ResourceBodyEditor(
  {
    value,
    onChange,
    extensions,
    placeholder,
    error,
    onBlur,
    disabled = false,
    className,
    view,
    onViewChange,
    'aria-label': ariaLabel,
    'data-testid': testId,
    id,
    'aria-describedby': describedBy,
    'aria-invalid': invalid,
  },
  ref
) {
  const [ownView, setOwnView] = useState<BodyEditorView>('write')
  const [, setStoredMode] = useBodyViewMode()
  const requested = view ?? ownView
  // A Render option that is not offered must never be the active one — an
  // extension can be withdrawn (the prompt offers it only while editing) long
  // after the view was set.
  const activeView =
    requested === 'render' && !extensions?.render ? 'write' : requested

  const handleViewChange = (next: string) => {
    const nextView = next as BodyEditorView
    // Render is the prompt's own view, not a way of reading a body: it is
    // never stored.
    if (nextView !== 'render') {
      setStoredMode(nextView === 'write' ? 'raw' : 'rendered')
    }
    if (onViewChange) onViewChange(nextView)
    else setOwnView(nextView)
  }

  const options: SegmentedOption[] = [
    { value: 'preview', label: 'Rendered' },
    { value: 'write', label: 'Raw' },
  ]
  if (extensions?.render) {
    options.push({
      value: 'render',
      label: 'Render',
      disabled: extensions.render.disabled,
    })
  }

  const slot = { id, 'aria-describedby': describedBy, 'aria-invalid': invalid }

  return (
    <div className={cn('space-y-4', className)}>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {extensions?.mentions && activeView !== 'render' && (
          <Badge variant="outline" className="gap-1">
            <Wand2 className="size-3" />
            Type @ to reference prompts
          </Badge>
        )}
        {extensions?.templates && (
          <Button
            variant="outline"
            size="sm"
            type="button"
            onClick={extensions.templates}
          >
            <Download className="mr-2 size-3.5" />
            Load template
          </Button>
        )}
        <SegmentedControl
          size="sm"
          aria-label="Body view"
          options={options}
          value={activeView}
          onChange={handleViewChange}
        />
      </div>

      {/*
        Only the active pane is mounted, under `role="tabpanel"`, exactly as in
        `ResourceBody`: the e2e suite's `getByRole('tabpanel')` is strict, so a
        second mounted panel would break it.
      */}
      <div role="tabpanel" aria-label="Resource body">
        {activeView === 'write' &&
          (extensions?.mentions ? (
            // The mention textarea renders the invalid state AND the message
            // itself, from its own `error` prop — hand it the error, and do
            // not render a second message beside it.
            <PromptMentionTextarea
              {...slot}
              ref={ref}
              frameless
              data-testid={testId}
              aria-label={ariaLabel}
              value={value}
              onChange={onChange}
              onBlur={onBlur}
              error={error}
              disabled={disabled}
              placeholder={placeholder}
              rows={BODY_EDITOR_MIN_ROWS}
              excludeCurrentPrompt={extensions.mentions.excludeCurrentPrompt}
              className={WRITE_TEXTAREA_CLASS}
            />
          ) : (
            <>
              <Textarea
                {...slot}
                ref={ref}
                data-testid={testId}
                aria-label={ariaLabel}
                value={value}
                onBlur={onBlur}
                disabled={disabled}
                placeholder={placeholder}
                rows={BODY_EDITOR_MIN_ROWS}
                className={cn(
                  WRITE_TEXTAREA_CLASS,
                  error && 'ring-destructive ring-2'
                )}
                onChange={event => {
                  onChange(event.target.value)
                }}
              />
              {error && (
                <p className="text-destructive mt-2 flex items-center gap-1 text-sm">
                  <AlertCircle className="size-4" />
                  {error}
                </p>
              )}
            </>
          ))}

        {activeView === 'preview' && (
          <div className={BODY_EDITOR_MIN_HEIGHT}>
            <MarkdownRenderer
              content={value || 'Nothing to preview yet…'}
              syntaxTheme="auto"
              className="reading-body"
            />
          </div>
        )}

        {activeView === 'render' && extensions?.render?.content}
      </div>
    </div>
  )
})
