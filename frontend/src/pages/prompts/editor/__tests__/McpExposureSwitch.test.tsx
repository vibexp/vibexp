import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FormProvider, useForm } from 'react-hook-form'

import { McpExposureSwitch } from '../McpExposureSwitch'

function Harness({
  status,
  value = false,
  onChange = vi.fn(),
}: Readonly<{
  status: string
  value?: boolean
  onChange?: (next: boolean) => void
}>) {
  const form = useForm({ defaultValues: { status } })
  return (
    <FormProvider {...form}>
      <McpExposureSwitch value={value} onChange={onChange} />
    </FormProvider>
  )
}

describe('McpExposureSwitch', () => {
  it('offers nothing to toggle on a draft, and says how to change that', () => {
    render(<Harness status="draft" />)

    expect(screen.queryByTestId('prompt-mcp-expose')).not.toBeInTheDocument()
    expect(screen.getByText('Not exposed')).toHaveAttribute(
      'title',
      'Move the prompt out of draft to make it available in MCP'
    )
  })

  // "Not draft" rather than "published": a team's custom status (#1263) is a
  // live prompt too, and the server exposes it.
  it.each(['published', 'in-review'])(
    'renders the switch for the non-draft status %s',
    status => {
      render(<Harness status={status} value />)

      expect(screen.getByTestId('prompt-mcp-expose')).toBeChecked()
      expect(screen.queryByText('Not exposed')).not.toBeInTheDocument()
    }
  )

  it('reports a toggle on a custom status', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Harness status="in-review" onChange={onChange} />)

    await user.click(screen.getByRole('switch', { name: 'Available in MCP' }))

    expect(onChange).toHaveBeenCalledWith(true)
  })
})
