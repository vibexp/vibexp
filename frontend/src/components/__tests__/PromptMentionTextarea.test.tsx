import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import { PromptMentionTextarea } from '@/components/PromptMentionTextarea'

const trackEvent = vi.hoisted(() => vi.fn())

vi.mock('@/hooks', () => ({
  useAnalytics: () => ({ trackEvent }),
}))

// The picker is a dialog over the prompts API; the textarea's own contract is
// what this file is about, so reduce it to whether it was asked to open.
vi.mock('@/components/PromptTemplateLoader', () => ({
  PromptTemplateLoader: ({
    isOpen,
    excludeCurrentPrompt,
  }: {
    isOpen: boolean
    excludeCurrentPrompt?: string
  }) =>
    isOpen ? (
      <div data-testid="template-loader" data-exclude={excludeCurrentPrompt} />
    ) : null,
}))

/**
 * `ResourceBodyEditor` swaps this component in for its `mentions` extension,
 * so it is the leaf `<textarea>` of a body field — and the editor's own suite
 * mocks it, which means only these cases can prove the props it is handed
 * actually land. Dropping any one of them is invisible everywhere else.
 */
describe('PromptMentionTextarea — the form-control contract', () => {
  it('lands the accessible name and the FormControl slot ids on the textarea', () => {
    render(
      <PromptMentionTextarea
        value=""
        onChange={vi.fn()}
        aria-label="Content"
        id="body-field"
        aria-describedby="body-description"
        aria-invalid
      />
    )

    const textarea = screen.getByLabelText('Content')
    expect(textarea.tagName).toBe('TEXTAREA')
    expect(textarea).toHaveAttribute('id', 'body-field')
    expect(textarea).toHaveAttribute('aria-describedby', 'body-description')
    expect(textarea).toHaveAttribute('aria-invalid', 'true')
  })

  it('disables the textarea, and looks disabled while it is', () => {
    render(<PromptMentionTextarea value="" onChange={vi.fn()} disabled />)

    const textarea = screen.getByRole('textbox')
    expect(textarea).toBeDisabled()
    // The plain pane's leaf gets these from shadcn's Textarea; this one has to
    // carry them itself, or a body locked mid-save still looks editable.
    expect(textarea.className).toContain('disabled:cursor-not-allowed')
    expect(textarea.className).toContain('disabled:opacity-50')
  })

  it('reports blur, so react-hook-form can mark the field touched', async () => {
    const user = userEvent.setup()
    const onBlur = vi.fn()
    render(
      <PromptMentionTextarea value="" onChange={vi.fn()} onBlur={onBlur} />
    )

    await user.click(screen.getByRole('textbox'))
    await user.tab()

    expect(onBlur.mock.calls).toHaveLength(1)
  })

  it('renders the invalid state itself, message and border together', () => {
    render(
      <PromptMentionTextarea
        value=""
        onChange={vi.fn()}
        error="Body is required"
      />
    )

    expect(screen.getByText('Body is required')).toBeInTheDocument()
    const textarea = screen.getByRole('textbox')
    expect(textarea.className).toContain('border-destructive')
    // The neutral border must be REPLACED, not merely outranked: this class
    // list is a template literal, so tailwind-merge never sees it.
    expect(textarea.className).not.toContain('border-input')
  })

  it('keeps the caller class alongside its own', () => {
    render(
      <PromptMentionTextarea
        value=""
        onChange={vi.fn()}
        className="field-sizing-content min-h-96"
      />
    )

    const textarea = screen.getByRole('textbox')
    expect(textarea.className).toContain('field-sizing-content')
    expect(textarea.className).toContain('min-h-96')
  })

  it('resolves a forwarded ref to the same textarea it types into', async () => {
    const user = userEvent.setup()
    const ref = createRef<HTMLTextAreaElement>()
    const onChange = vi.fn()
    render(<PromptMentionTextarea ref={ref} value="" onChange={onChange} />)

    const textarea = screen.getByRole('textbox')
    expect(ref.current).toBe(textarea)

    // The internal caret ref must survive being composed with the external one.
    await user.type(textarea, 'a')
    expect(onChange).toHaveBeenLastCalledWith('a')
  })

  it('still opens the picker on @, excluding the prompt being edited', async () => {
    const user = userEvent.setup()
    render(
      <PromptMentionTextarea
        value=""
        onChange={vi.fn()}
        excludeCurrentPrompt="my-prompt"
      />
    )

    await user.type(screen.getByRole('textbox'), '@')

    const loader = screen.getByTestId('template-loader')
    expect(loader).toHaveAttribute('data-exclude', 'my-prompt')
  })
})
