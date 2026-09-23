import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import { RestApis } from '../RestApis'

describe('REST APIs integration page', () => {
  // The test env sets an absolute VITE_API_BASE_URL (vitest.config.ts), i.e.
  // the split-origin topology of local dev: the spec is served by the
  // backend, not by the SPA's origin (#1129).
  const origin = 'https://api.vibexp.io'

  it('renders without any team or auth provider', () => {
    render(<RestApis />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'REST APIs' })
    ).toBeInTheDocument()
  })

  it("shows both schema URLs built from the backend's origin", () => {
    render(<RestApis />)

    expect(screen.getByText(`${origin}/openapi.yaml`)).toBeInTheDocument()
    expect(screen.getByText(`${origin}/openapi.json`)).toBeInTheDocument()
    expect(
      screen.queryByText(`${window.location.origin}/openapi.yaml`)
    ).not.toBeInTheDocument()
  })

  describe('when the API is same-origin (combined image)', () => {
    afterEach(() => {
      vi.unstubAllEnvs()
    })

    it.each([['/api/v1'], ['']])(
      'builds the schema URLs from the browsing origin for base %j',
      base => {
        vi.stubEnv('VITE_API_BASE_URL', base)
        render(<RestApis />)

        const browsing = window.location.origin
        expect(screen.getByText(`${browsing}/openapi.yaml`)).toBeInTheDocument()
        expect(
          screen.getByRole('link', { name: 'Open JSON schema' })
        ).toHaveAttribute('href', `${browsing}/openapi.json`)
      }
    )
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
