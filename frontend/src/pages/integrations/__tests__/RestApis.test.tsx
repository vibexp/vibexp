import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import { RestApis } from '../RestApis'

describe('REST APIs integration page', () => {
  const origin = window.location.origin

  it('renders without any team or auth provider', () => {
    render(<RestApis />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'REST APIs' })
    ).toBeInTheDocument()
  })

  it('shows both schema URLs built from the current origin', () => {
    render(<RestApis />)

    expect(screen.getByText(`${origin}/openapi.yaml`)).toBeInTheDocument()
    expect(screen.getByText(`${origin}/openapi.json`)).toBeInTheDocument()
  })

  it.each([
    ['YAML', '/openapi.yaml'],
    ['JSON', '/openapi.json'],
  ])('opens the %s schema in a new tab', (label, path) => {
    render(<RestApis />)

    const link = screen.getByRole('link', { name: `Open ${label} schema` })
    expect(link).toHaveAttribute('href', `${origin}${path}`)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('copies a schema URL to the clipboard and confirms it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    render(<RestApis />)

    const button = screen.getByRole('button', {
      name: 'Copy JSON schema URL',
    })
    fireEvent.click(button)

    expect(writeText).toHaveBeenCalledWith(`${origin}/openapi.json`)
    await waitFor(() => {
      expect(button).toHaveTextContent('Copied')
    })
  })

  it('does not claim success when the clipboard write fails', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    Object.assign(navigator, { clipboard: { writeText } })
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined)
    render(<RestApis />)

    const button = screen.getByRole('button', {
      name: 'Copy YAML schema URL',
    })
    fireEvent.click(button)

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalled()
    })
    expect(writeText).toHaveBeenCalledWith(`${origin}/openapi.yaml`)
    expect(button).not.toHaveTextContent('Copied')
    consoleError.mockRestore()
  })
})
