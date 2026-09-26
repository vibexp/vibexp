import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef, forwardRef, useState } from 'react'

import {
  RAW_BODY_CLASS,
  useBodyViewMode,
} from '@/components/patterns/reading-page'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { storage } from '@/utils/storage'

import type { BodyEditorView } from '../ResourceBodyEditor'
import {
  BODY_EDITOR_MIN_HEIGHT,
  BODY_EDITOR_MIN_ROWS,
  ResourceBodyEditor,
} from '../ResourceBodyEditor'

// marked/DOMPurify/mermaid are heavy in jsdom; Rendered only has to prove
// which text reaches the SAME renderer, with the same props, the detail body
// uses.
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({
    content,
    className,
    syntaxTheme,
  }: {
    content: string
    className?: string
    syntaxTheme?: string
  }) => (
    <div
      data-testid="markdown-renderer"
      className={className}
      data-syntax-theme={syntaxTheme}
    >
      {content}
    </div>
  ),
}))

// The real one pulls in analytics and the template-picker dialog. The stand-in
// keeps the shape that matters here: ONE `<textarea>` leaf that every prop —
// including the ref — has to reach, and the component's own error message.
vi.mock('@/components/PromptMentionTextarea', () => ({
  PromptMentionTextarea: forwardRef<
    HTMLTextAreaElement,
    {
      value: string
      onChange: (next: string) => void
      onBlur?: () => void
      className?: string
      rows?: number
      error?: string
      disabled?: boolean
      'data-testid'?: string
      'aria-label'?: string
      id?: string
      'aria-describedby'?: string
      'aria-invalid'?: boolean
      excludeCurrentPrompt?: string
      frameless?: boolean
    }
  >(function PromptMentionTextarea(props, ref) {
    const {
      value,
      onChange,
      onBlur,
      className,
      rows,
      error,
      disabled,
      'data-testid': testId,
      'aria-label': ariaLabel,
      id,
      'aria-describedby': describedBy,
      'aria-invalid': invalid,
      excludeCurrentPrompt,
      frameless,
    } = props
    return (
      <div>
        <textarea
          ref={ref}
          id={id}
          aria-label={ariaLabel}
          aria-describedby={describedBy}
          aria-invalid={invalid}
          data-testid={testId}
          data-mention-textarea="true"
          data-exclude={excludeCurrentPrompt}
          data-frameless={frameless ? 'true' : undefined}
          className={className}
          rows={rows}
          disabled={disabled}
          value={value}
          onBlur={onBlur}
          onChange={event => {
            onChange(event.target.value)
          }}
        />
        {error && <p>{error}</p>}
      </div>
    )
  }),
}))

const BODY = '# Heading\n\nSome **markdown** body.'

function writeArea() {
  return screen.getByRole('textbox')
}

function storedMode() {
  return renderHook(() => useBodyViewMode()).result.current[0]
}

beforeEach(() => {
  storage.remove(STORAGE_KEYS.BODY_VIEW_RAW)
})

describe('ResourceBodyEditor', () => {
  describe('with no extensions', () => {
    it('offers Rendered and Raw only, with Raw active', () => {
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      expect(screen.getByRole('tablist', { name: 'Body view' })).toBeVisible()
      expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByRole('tab', { name: 'Rendered' })).toBeInTheDocument()
      expect(
        screen.queryByRole('tab', { name: 'Render' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /Load template/ })
      ).not.toBeInTheDocument()
      expect(screen.queryByText(/Type @ to reference/)).not.toBeInTheDocument()
    })

    it('uses a plain textarea, not the prompt mention one', () => {
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      expect(writeArea()).not.toHaveAttribute('data-mention-textarea')
    })

    it('reports every keystroke through onChange', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      render(<ResourceBodyEditor value="" onChange={onChange} />)

      await user.type(writeArea(), 'abc')

      expect(onChange.mock.calls).toHaveLength(3)
      expect(onChange).toHaveBeenLastCalledWith('c')
    })

    it('renders through the same MarkdownRenderer, with the same props, as the detail body', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      const rendered = screen.getByTestId('markdown-renderer')
      expect(rendered).toHaveTextContent('Some **markdown** body.')
      expect(rendered).toHaveClass('reading-body')
      expect(rendered).toHaveAttribute('data-syntax-theme', 'auto')
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    })

    it('says there is nothing to preview when the body is empty', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value="" onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      expect(screen.getByTestId('markdown-renderer')).toHaveTextContent(
        'Nothing to preview yet'
      )
    })

    it('renders the inline error once, under the Raw pane', () => {
      render(
        <ResourceBodyEditor
          value=""
          onChange={vi.fn()}
          error="Body is required"
        />
      )

      expect(screen.getByText('Body is required')).toBeInTheDocument()
      // There is no border left to colour, so the invalid state is a ring.
      expect(writeArea()).toHaveClass('ring-2', 'ring-destructive')
    })

    it('forwards the FormControl slot props to the textarea leaf', () => {
      render(
        <ResourceBodyEditor
          value=""
          onChange={vi.fn()}
          id="body-field"
          aria-describedby="body-description"
          aria-invalid
          aria-label="Content"
        />
      )

      const textarea = screen.getByLabelText('Content')
      expect(textarea).toHaveAttribute('id', 'body-field')
      expect(textarea).toHaveAttribute('aria-describedby', 'body-description')
      expect(textarea).toHaveAttribute('aria-invalid', 'true')
    })
  })

  describe('sizing', () => {
    it('grows with its content instead of scrolling inside a fixed box', () => {
      // jsdom has no layout engine, so the assertion is on the mechanism: the
      // pane declares a content-driven intrinsic height and a shared minimum,
      // and sets no height of its own.
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      const textarea = writeArea()
      expect(textarea.className).toContain('field-sizing-content')
      expect(textarea.className).toContain(BODY_EDITOR_MIN_HEIGHT)
      expect(textarea).toHaveAttribute('rows', String(BODY_EDITOR_MIN_ROWS))
      expect(textarea.style.height).toBe('')
    })

    it('uses no arbitrary height value anywhere in the editor', () => {
      const { container } = render(
        <ResourceBodyEditor value={BODY} onChange={vi.fn()} />
      )

      const classes = [...container.querySelectorAll('[class]')]
        .map(node => node.className)
        .join(' ')
      expect(classes).not.toMatch(/min-h-\[|max-h-\[|\bh-\[/)
    })

    it('starts the Rendered pane at the same shared minimum', async () => {
      const user = userEvent.setup()
      const { container } = render(
        <ResourceBodyEditor value={BODY} onChange={vi.fn()} />
      )

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      expect(
        container.querySelector(`.${BODY_EDITOR_MIN_HEIGHT}`)
      ).toBeInTheDocument()
    })
  })

  describe('mentions extension', () => {
    it('swaps in the prompt mention textarea and announces the shortcut', () => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ mentions: { excludeCurrentPrompt: 'my-prompt' } }}
        />
      )

      const textarea = writeArea()
      expect(textarea).toHaveAttribute('data-mention-textarea', 'true')
      expect(textarea).toHaveAttribute('data-exclude', 'my-prompt')
      expect(
        screen.getByText(/Type @ to reference prompts/)
      ).toBeInTheDocument()
    })

    it('gives the mention textarea the same sizing contract', () => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ mentions: {} }}
        />
      )

      const textarea = writeArea()
      expect(textarea.className).toContain('field-sizing-content')
      expect(textarea.className).toContain(BODY_EDITOR_MIN_HEIGHT)
      expect(textarea).toHaveAttribute('rows', String(BODY_EDITOR_MIN_ROWS))
    })
  })

  describe('form-control wiring', () => {
    // Every one of these reached the leaf on the plain path and was dropped on
    // the mentions path in the first cut — a control that stays editable
    // mid-save and a label pointing at nothing, both silently.
    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)(
      'forwards the ref to the %s textarea leaf',
      (_case, extensions) => {
        const ref = createRef<HTMLTextAreaElement>()
        render(
          <ResourceBodyEditor
            ref={ref}
            value={BODY}
            onChange={vi.fn()}
            extensions={extensions}
          />
        )

        expect(ref.current).toBe(writeArea())
      }
    )

    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)('disables the %s textarea', (_case, extensions) => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          disabled
          extensions={extensions}
        />
      )

      expect(writeArea()).toBeDisabled()
    })

    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)(
      'names the %s textarea and carries the slot ids',
      (_case, extensions) => {
        render(
          <ResourceBodyEditor
            value={BODY}
            onChange={vi.fn()}
            aria-label="Content"
            id="body-field"
            aria-describedby="body-description"
            aria-invalid
            extensions={extensions}
          />
        )

        const textarea = screen.getByLabelText('Content')
        expect(textarea).toHaveAttribute('id', 'body-field')
        expect(textarea).toHaveAttribute('aria-describedby', 'body-description')
        expect(textarea).toHaveAttribute('aria-invalid', 'true')
      }
    )

    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)(
      'reports blur on the %s textarea',
      async (_case, extensions) => {
        const user = userEvent.setup()
        const onBlur = vi.fn()
        render(
          <ResourceBodyEditor
            value={BODY}
            onChange={vi.fn()}
            onBlur={onBlur}
            extensions={extensions}
          />
        )

        await user.click(writeArea())
        await user.tab()

        expect(onBlur.mock.calls).toHaveLength(1)
      }
    )

    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)(
      'shows the error exactly once on the %s pane',
      (_case, extensions) => {
        render(
          <ResourceBodyEditor
            value=""
            onChange={vi.fn()}
            error="Body is required"
            extensions={extensions}
          />
        )

        // The mention textarea renders its own message from `error`; the editor
        // must hand it over rather than add a second one beside it.
        expect(screen.getAllByText('Body is required')).toHaveLength(1)
      }
    )
  })

  describe('render extension', () => {
    const RENDER_EXT = {
      render: { content: <p>rendered output</p> },
    }

    it('adds a Render option that shows the panel it was handed', async () => {
      const user = userEvent.setup()
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={RENDER_EXT}
        />
      )

      await user.click(screen.getByRole('tab', { name: 'Render' }))

      expect(screen.getByText('rendered output')).toBeInTheDocument()
    })

    it('keeps the option present but disabled while the panel is not ready', async () => {
      const user = userEvent.setup()
      const onViewChange = vi.fn()
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          view="write"
          onViewChange={onViewChange}
          extensions={{ render: { content: <p>x</p>, disabled: true } }}
        />
      )

      const option = screen.getByRole('tab', { name: 'Render' })
      expect(option).toBeDisabled()
      expect(option).toHaveAttribute('aria-disabled', 'true')
      await user.click(option)
      expect(onViewChange).not.toHaveBeenCalled()
    })

    it('falls back to Raw when the extension is withdrawn under it', () => {
      // The prompt offers Render only while editing, so the active view can
      // outlive the tab that produced it.
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          view="render"
          onViewChange={vi.fn()}
        />
      )

      expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(writeArea()).toHaveValue(BODY)
    })
  })

  describe('templates extension', () => {
    it('renders the button and calls back on click', async () => {
      const user = userEvent.setup()
      const onLoad = vi.fn()
      render(
        <ResourceBodyEditor
          value=""
          onChange={vi.fn()}
          extensions={{ templates: onLoad }}
        />
      )

      await user.click(screen.getByRole('button', { name: /Load template/ }))

      expect(onLoad.mock.calls).toHaveLength(1)
    })
  })

  describe('view state', () => {
    it('keeps its own view when the caller does not supply one', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      expect(screen.getByRole('tab', { name: 'Rendered' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    })

    it('defers to a controlled view', async () => {
      const user = userEvent.setup()
      const onViewChange = vi.fn()

      function Controlled() {
        const [view] = useState<BodyEditorView>('write')
        return (
          <ResourceBodyEditor
            value={BODY}
            onChange={vi.fn()}
            view={view}
            onViewChange={onViewChange}
          />
        )
      }
      render(<Controlled />)

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))

      // The caller owns the state: it was told, and nothing moved on its own.
      expect(onViewChange).toHaveBeenCalledWith('preview')
      expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    })
  })

  describe('matches the reading body (#1178)', () => {
    it.each([
      ['plain', undefined],
      ['mentions', { mentions: {} }],
    ] as const)(
      'gives the %s Raw pane the Raw block surface and no border',
      (_case, extensions) => {
        render(
          <ResourceBodyEditor
            value={BODY}
            onChange={vi.fn()}
            extensions={extensions}
          />
        )

        const textarea = writeArea()
        expect(textarea).toHaveClass(...RAW_BODY_CLASS.split(' '), 'border-0')
        // Textarea's own `text-base md:text-sm` must not win the type size back.
        expect(textarea).not.toHaveClass('text-base')
        expect(textarea).not.toHaveClass('bg-background')
      }
    )

    it('asks the mention textarea to drop its own frame', () => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ mentions: {} }}
        />
      )

      expect(writeArea()).toHaveAttribute('data-frameless', 'true')
    })

    it('renders no card around either pane', async () => {
      const user = userEvent.setup()
      const { container } = render(
        <ResourceBodyEditor value={BODY} onChange={vi.fn()} />
      )

      expect(container.querySelector('.bg-card')).toBeNull()
      await user.click(screen.getByRole('tab', { name: 'Rendered' }))
      expect(container.querySelector('.bg-card')).toBeNull()
    })

    it('mounts exactly one tabpanel', async () => {
      const user = userEvent.setup()
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ render: { content: <p>out</p> } }}
        />
      )

      expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
      await user.click(screen.getByRole('tab', { name: 'Rendered' }))
      expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
      await user.click(screen.getByRole('tab', { name: 'Render' }))
      expect(screen.getAllByRole('tabpanel')).toHaveLength(1)
    })

    it('opens on Raw even when the stored preference is Rendered', () => {
      expect(storedMode()).toBe('rendered')

      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      expect(screen.getByRole('tab', { name: 'Raw' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(writeArea()).toHaveValue(BODY)
    })

    it('writes Rendered and Raw through to the shared reading preference', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Raw' }))
      expect(storedMode()).toBe('raw')

      await user.click(screen.getByRole('tab', { name: 'Rendered' }))
      expect(storedMode()).toBe('rendered')
    })

    it('writes the preference through for a controlled caller too', async () => {
      const user = userEvent.setup()
      const onViewChange = vi.fn()
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          view="preview"
          onViewChange={onViewChange}
        />
      )

      await user.click(screen.getByRole('tab', { name: 'Raw' }))

      expect(onViewChange).toHaveBeenCalledWith('write')
      expect(storedMode()).toBe('raw')
    })

    it('never stores Render', async () => {
      const user = userEvent.setup()
      act(() => {
        storage.set(STORAGE_KEYS.BODY_VIEW_RAW, 'true')
      })
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ render: { content: <p>out</p> } }}
        />
      )

      await user.click(screen.getByRole('tab', { name: 'Render' }))

      expect(storedMode()).toBe('raw')
    })

    it('puts the shortcut badge and template button left of the switch', () => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ mentions: {}, templates: vi.fn() }}
        />
      )

      const badge = screen.getByText('Type @ to reference prompts')
      const button = screen.getByRole('button', { name: /Load template/ })
      const toggle = screen.getByRole('tablist', { name: 'Body view' })
      // One row, in reading order: badge, template button, then the switch.
      expect(badge.parentElement).toBe(toggle.parentElement)
      expect(button.parentElement).toBe(toggle.parentElement)
      expect(
        badge.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
      expect(
        button.compareDocumentPosition(toggle) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
    })
  })
})
