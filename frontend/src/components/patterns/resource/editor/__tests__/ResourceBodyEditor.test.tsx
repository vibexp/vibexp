import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import type { BodyEditorView } from '../ResourceBodyEditor'
import {
  BODY_EDITOR_MIN_HEIGHT,
  BODY_EDITOR_MIN_ROWS,
  ResourceBodyEditor,
} from '../ResourceBodyEditor'

// marked/DOMPurify/mermaid are heavy in jsdom; Preview only has to prove which
// text reaches the SAME renderer the detail body uses.
vi.mock('@/components/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => (
    <div data-testid="markdown-renderer">{content}</div>
  ),
}))

// The real one pulls in analytics and the template-picker dialog. The editor's
// contract with it is narrow — which props it hands over — so record them.
vi.mock('@/components/PromptMentionTextarea', () => ({
  PromptMentionTextarea: ({
    value,
    onChange,
    className,
    rows,
    'data-testid': testId,
    excludeCurrentPrompt,
  }: {
    value: string
    onChange: (next: string) => void
    className?: string
    rows?: number
    'data-testid'?: string
    excludeCurrentPrompt?: string
  }) => (
    <textarea
      data-testid={testId}
      data-mention-textarea="true"
      data-exclude={excludeCurrentPrompt}
      className={className}
      rows={rows}
      value={value}
      onChange={event => {
        onChange(event.target.value)
      }}
    />
  ),
}))

const BODY = '# Heading\n\nSome **markdown** body.'

function writeArea() {
  return screen.getByRole('textbox')
}

describe('ResourceBodyEditor', () => {
  describe('with no extensions', () => {
    it('offers Write and Preview only, with Write active', () => {
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      expect(screen.getByRole('tab', { name: 'Write' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getByRole('tab', { name: 'Preview' })).toBeInTheDocument()
      expect(
        screen.queryByRole('tab', { name: /Render/ })
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

    it('previews through the same MarkdownRenderer the detail body uses', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value={BODY} onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Preview' }))

      expect(screen.getByTestId('markdown-renderer')).toHaveTextContent(
        'Some **markdown** body.'
      )
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    })

    it('says there is nothing to preview when the body is empty', async () => {
      const user = userEvent.setup()
      render(<ResourceBodyEditor value="" onChange={vi.fn()} />)

      await user.click(screen.getByRole('tab', { name: 'Preview' }))

      expect(screen.getByTestId('markdown-renderer')).toHaveTextContent(
        'Nothing to preview yet'
      )
    })

    it('renders the inline error once, under the write pane', () => {
      render(
        <ResourceBodyEditor
          value=""
          onChange={vi.fn()}
          error="Body is required"
        />
      )

      expect(screen.getByText('Body is required')).toBeInTheDocument()
      expect(writeArea().className).toContain('border-destructive')
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

    it('starts the preview pane at the same shared minimum', async () => {
      const user = userEvent.setup()
      const { container } = render(
        <ResourceBodyEditor value={BODY} onChange={vi.fn()} />
      )

      await user.click(screen.getByRole('tab', { name: 'Preview' }))

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

  describe('render extension', () => {
    const RENDER_EXT = {
      render: { content: <p>rendered output</p> },
    }

    it('adds a Render tab that shows the panel it was handed', async () => {
      const user = userEvent.setup()
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={RENDER_EXT}
        />
      )

      await user.click(screen.getByRole('tab', { name: /Render/ }))

      expect(screen.getByText('rendered output')).toBeInTheDocument()
    })

    it('disables the trigger while the panel is not ready', () => {
      render(
        <ResourceBodyEditor
          value={BODY}
          onChange={vi.fn()}
          extensions={{ render: { content: <p>x</p>, disabled: true } }}
        />
      )

      expect(screen.getByRole('tab', { name: /Render/ })).toBeDisabled()
    })

    it('falls back to Write when the extension is withdrawn under it', () => {
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

      expect(screen.getByRole('tab', { name: 'Write' })).toHaveAttribute(
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

      await user.click(screen.getByRole('tab', { name: 'Preview' }))

      expect(screen.getByRole('tab', { name: 'Preview' })).toHaveAttribute(
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

      await user.click(screen.getByRole('tab', { name: 'Preview' }))

      // The caller owns the state: it was told, and nothing moved on its own.
      expect(onViewChange).toHaveBeenCalledWith('preview')
      expect(screen.getByRole('tab', { name: 'Write' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    })
  })
})
