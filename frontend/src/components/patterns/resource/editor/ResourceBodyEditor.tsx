import { AlertCircle, Download, Wand2 } from 'lucide-react'
import type { ReactNode } from 'react'
import type { RefObject } from 'react'
import { forwardRef, useId, useLayoutEffect, useRef, useState } from 'react'

import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import type { BodyFormat } from '@/components/patterns/reading-page'
import { PromptMentionTextarea } from '@/components/PromptMentionTextarea'
import type { SegmentedOption } from '@/components/SegmentedControl'
import { SegmentedControl } from '@/components/SegmentedControl'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { RAW_EDITOR_TAB_HINT, useRawEditorKeys } from './useRawEditorKeys'

/**
 * The one minimum height every body pane shares.
 *
 * A minimum is not the thing #914 removes — four different magic numbers are
 * (22 rows, 24 rows, 30 rows, and a hard 600px floor on the prompt). One
 * exported constant is what makes "the editor is the same size everywhere" a
 * fact rather than a coincidence, and it is what the tests assert against, so
 * the two cannot drift. It is a share of the viewport (`min-h-editor`, 60vh,
 * defined in `styles/index.css`), so the field opens as a page to write on
 * rather than a small window onto a long document.
 */
export const BODY_EDITOR_MIN_HEIGHT = 'min-h-editor'

/**
 * The floor in lines, for the browsers where it bites before the viewport
 * one. Deliberately small, so {@link BODY_EDITOR_MIN_HEIGHT} is what governs
 * and both panes — Raw and Rendered — start the same height.
 */
export const BODY_EDITOR_MIN_ROWS = 12

/**
 * The Raw pane: an ordinary input surface, sized to its content.
 *
 * Surface: the page background, a 1px input border and the standard focus
 * ring — the same edge every other field on the form has. (It used to share
 * the Raw `<pre>`'s grey fill, which read as a disabled field.)
 *
 * Type: monospace at 14px/1.6 with a two-space tab, soft-wrapped at the field
 * width, so markdown and code line up and a long line never scrolls sideways.
 *
 * Size: `field-sizing: content` makes the textarea's height track what is
 * typed into it, so a long body pushes the PAGE down. There is no inner
 * scrollbar and no resize handle — a drag writes an inline `height` that
 * outranks `field-sizing` and would bring the inner scrollbar back. Where the
 * property is unsupported, {@link useAutoGrow} sets the height from script.
 */
const WRITE_TEXTAREA_CLASS = `${BODY_EDITOR_MIN_HEIGHT} block w-full field-sizing-content resize-none overflow-hidden rounded-md border border-input bg-background p-4 font-mono text-sm leading-[1.6] text-foreground [tab-size:2] whitespace-pre-wrap break-words ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50`

/** Spellcheck underlines code, placeholders and markdown syntax; off here. */
const RAW_FIELD_INPUT_PROPS = {
  spellCheck: false,
  autoCorrect: 'off',
  autoCapitalize: 'off',
} as const

/**
 * Grows a textarea to its content where `field-sizing: content` is missing,
 * so the page scrolls rather than the field in every browser.
 */
function useAutoGrow(
  ref: RefObject<HTMLTextAreaElement | null>,
  value: string,
  active: boolean
): void {
  useLayoutEffect(() => {
    const el = ref.current
    if (!el || !active) return
    if (typeof CSS !== 'undefined' && CSS.supports('field-sizing', 'content')) {
      return
    }
    el.style.height = 'auto'
    // No layout (jsdom, a hidden pane): leave the CSS minimum in charge.
    if (el.scrollHeight > 0) el.style.height = `${String(el.scrollHeight)}px`
  }, [ref, value, active])
}

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
  /**
   * Renders `{{placeholders}}` in the Rendered pane as chips (or as these
   * values, where one is given) instead of as literal text.
   */
  placeholders?: Record<string, string>
  /** Called by a "Load template" button rendered beside the switch. */
  templates?: () => void
}

interface ResourceBodyEditorBaseProps {
  value: string
  onChange: (next: string) => void
  /**
   * The resource's title as the form currently holds it. As on the reading
   * page, a leading `# Heading` that only repeats it is left out of the
   * Rendered pane, so the preview matches what the saved resource will show.
   */
  title?: string
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
 * Rendered) so a controlled caller is unaffected. Edit always opens on Raw, and
 * what is chosen here stays here: it never changes how a reading page opens
 * (those always open on Rendered).
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
    title,
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
  const requested = view ?? ownView
  // A Render option that is not offered must never be the active one — an
  // extension can be withdrawn (the prompt offers it only while editing) long
  // after the view was set.
  const activeView =
    requested === 'render' && !extensions?.render ? 'write' : requested

  const handleViewChange = (next: string) => {
    const nextView = next as BodyEditorView
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
      // "Render" sat beside "Rendered" and read as the same thing; this view
      // is the body as an AI tool receives it.
      label: 'Resolved',
      title: 'With @references expanded, as an AI tool receives it',
      disabled: extensions.render.disabled,
    })
  }

  // The Raw field: its own node (to size it), merged with the form's ref.
  const fieldRef = useRef<HTMLTextAreaElement | null>(null)
  const setFieldRef = (node: HTMLTextAreaElement | null) => {
    fieldRef.current = node
    if (typeof ref === 'function') ref(node)
    else if (ref) ref.current = node
  }
  useAutoGrow(fieldRef, value, activeView === 'write')

  const keys = useRawEditorKeys(onChange)
  const tabHintId = useId()
  const handleBlur = () => {
    keys.onBlur()
    onBlur?.()
  }

  const slot = {
    id,
    // The Tab hint is announced with the field, after any form message.
    'aria-describedby': describedBy ? `${describedBy} ${tabHintId}` : tabHintId,
    'aria-invalid': invalid,
  }

  return (
    <div className={cn('space-y-4', className)}>
      <div className="reading-prose-width flex flex-wrap items-center justify-end gap-2">
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
        {activeView === 'write' && (
          <p id={tabHintId} className="sr-only">
            {RAW_EDITOR_TAB_HINT}
          </p>
        )}
        {activeView === 'write' &&
          (extensions?.mentions ? (
            // The mention textarea renders the invalid state AND the message
            // itself, from its own `error` prop — hand it the error, and do
            // not render a second message beside it.
            <PromptMentionTextarea
              {...slot}
              {...RAW_FIELD_INPUT_PROPS}
              ref={setFieldRef}
              frameless
              data-testid={testId}
              aria-label={ariaLabel}
              value={value}
              onChange={onChange}
              onBlur={handleBlur}
              onKeyDown={keys.onKeyDown}
              error={error}
              disabled={disabled}
              placeholder={placeholder}
              rows={BODY_EDITOR_MIN_ROWS}
              excludeCurrentPrompt={extensions.mentions.excludeCurrentPrompt}
              className={cn(
                WRITE_TEXTAREA_CLASS,
                error && 'border-destructive'
              )}
            />
          ) : (
            <>
              {/* A bare element, not `Textarea`: that component's own
                  `min-h-[80px]` would compete with the editor's minimum,
                  which class merging cannot see through. */}
              <textarea
                {...slot}
                {...RAW_FIELD_INPUT_PROPS}
                ref={setFieldRef}
                data-testid={testId}
                aria-label={ariaLabel}
                value={value}
                onBlur={handleBlur}
                onKeyDown={keys.onKeyDown}
                disabled={disabled}
                placeholder={placeholder}
                rows={BODY_EDITOR_MIN_ROWS}
                className={cn(
                  WRITE_TEXTAREA_CLASS,
                  error && 'border-destructive focus-visible:ring-destructive'
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
              hideLeadingTitle={title}
              placeholders={extensions?.placeholders}
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
