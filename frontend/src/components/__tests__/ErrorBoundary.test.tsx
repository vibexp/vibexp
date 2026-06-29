import { fireEvent, render, screen } from '@testing-library/react'

import { ErrorBoundary } from '../ErrorBoundary'

// A child that throws on first render, then renders fine after `shouldThrow`
// is flipped — lets us exercise the retry path.
function Boom({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error('boom')
  }
  return <div>recovered</div>
}

describe('ErrorBoundary', () => {
  // componentDidCatch logs via console.error; silence it and assert on it.
  let consoleErrorSpy: jest.SpyInstance

  beforeEach(() => {
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleErrorSpy.mockRestore()
  })

  it('renders children when no error is thrown', () => {
    render(
      <ErrorBoundary>
        <div>all good</div>
      </ErrorBoundary>
    )

    expect(screen.getByText('all good')).toBeInTheDocument()
  })

  it('renders the default fallback and logs the error when a child throws', () => {
    render(
      <ErrorBoundary>
        <Boom shouldThrow />
      </ErrorBoundary>
    )

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try Again' })
    ).toBeInTheDocument()
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      '[ErrorBoundary] React error caught:',
      expect.any(Error),
      expect.anything()
    )
  })

  it('renders a custom fallback when provided', () => {
    render(
      <ErrorBoundary fallback={<div>custom fallback</div>}>
        <Boom shouldThrow />
      </ErrorBoundary>
    )

    expect(screen.getByText('custom fallback')).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument()
  })

  it('calls the onError handler with the thrown error', () => {
    const onError = jest.fn()

    render(
      <ErrorBoundary onError={onError}>
        <Boom shouldThrow />
      </ErrorBoundary>
    )

    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError.mock.calls[0][0]).toBeInstanceOf(Error)
  })

  it('clears the error state and re-renders children after Try Again', () => {
    const { rerender } = render(
      <ErrorBoundary>
        <Boom shouldThrow />
      </ErrorBoundary>
    )

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()

    // Stop throwing, then retry — the boundary should render children again.
    rerender(
      <ErrorBoundary>
        <Boom shouldThrow={false} />
      </ErrorBoundary>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(screen.getByText('recovered')).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument()
  })
})
