/**
 * Error Boundary Component
 *
 * Catches JavaScript errors anywhere in the child component tree,
 * logs them to the console, and displays a fallback UI.
 *
 * No telemetry is bundled — errors are logged locally only.
 */

import type { ErrorInfo, ReactNode } from 'react'
import { Component } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
  onError?: (error: Error, errorInfo: ErrorInfo) => void
}

interface State {
  hasError: boolean
  error: Error | null
  errorInfo: ErrorInfo | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    }
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    // Update state so the next render will show the fallback UI
    return {
      hasError: true,
      error,
    }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Update state with error info
    this.setState({
      errorInfo,
    })

    // No error-tracking service is shipped — log to the console so the error
    // stays visible in dev tools and self-host logs.
    console.error('[ErrorBoundary] React error caught:', error, errorInfo)

    // Call custom error handler if provided
    if (this.props.onError) {
      try {
        this.props.onError(error, errorInfo)
      } catch (handlerError) {
        console.error(
          '[ErrorBoundary] Error in custom error handler:',
          handlerError
        )
      }
    }
  }

  handleRetry = () => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    })
  }

  render() {
    if (this.state.hasError) {
      // Custom fallback UI
      if (this.props.fallback) {
        return this.props.fallback
      }

      // Default fallback UI
      return (
        <div className="flex flex-col items-center justify-center min-h-[400px] p-8 bg-destructive/10 border border-destructive/30 rounded-lg">
          <div className="text-center max-w-2xl">
            <div className="text-destructive text-4xl mb-4">⚠️</div>
            <h3 className="text-xl font-semibold text-destructive mb-3">
              Something went wrong
            </h3>
            <p className="text-muted-foreground mb-6">
              An unexpected error occurred. You can try again or reload the
              page.
            </p>

            <div className="flex items-center justify-center gap-3">
              <button
                onClick={this.handleRetry}
                className="px-6 py-2.5 bg-destructive text-destructive-foreground rounded-md hover:bg-destructive/90 focus:outline-none focus:ring-2 focus:ring-destructive focus:ring-offset-2 transition-colors font-medium"
              >
                Try Again
              </button>
            </div>

            {import.meta.env.DEV && this.state.error && (
              <details className="mt-6 text-left">
                <summary className="cursor-pointer text-destructive font-medium hover:text-destructive/80">
                  Error Details (Development Mode)
                </summary>
                <div className="mt-3 p-4 bg-destructive/10 border border-destructive/30 rounded text-sm text-foreground overflow-auto">
                  <div className="font-semibold mb-2">Error:</div>
                  <pre className="mb-4 whitespace-pre-wrap">
                    {this.state.error.toString()}
                  </pre>

                  {this.state.errorInfo?.componentStack && (
                    <>
                      <div className="font-semibold mb-2">Component Stack:</div>
                      <pre className="whitespace-pre-wrap">
                        {this.state.errorInfo.componentStack}
                      </pre>
                    </>
                  )}
                </div>
              </details>
            )}
          </div>
        </div>
      )
    }

    return this.props.children
  }
}

/**
 * Higher-order component wrapper for Error Boundary
 */
export function withErrorBoundary<P extends object>(
  Component: React.ComponentType<P>,
  fallback?: ReactNode,
  onError?: (error: Error, errorInfo: ErrorInfo) => void
): React.ComponentType<P> {
  return function WrappedWithErrorBoundary(props: P) {
    return (
      <ErrorBoundary fallback={fallback} onError={onError}>
        <Component {...props} />
      </ErrorBoundary>
    )
  }
}
