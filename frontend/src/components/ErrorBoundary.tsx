/**
 * Error Boundary Component
 *
 * Catches JavaScript errors anywhere in the child component tree,
 * logs those errors, and displays a fallback UI.
 *
 * Integrated with Sentry for automatic error tracking.
 */

import * as Sentry from '@sentry/react'
import type { ErrorInfo, ReactNode } from 'react'
import { Component } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
  onError?: (error: Error, errorInfo: ErrorInfo) => void
  showDialog?: boolean // Whether to show Sentry user feedback dialog
}

interface State {
  hasError: boolean
  error: Error | null
  errorInfo: ErrorInfo | null
  eventId: string | null
}

/**
 * Error Boundary with Sentry Integration
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      eventId: null,
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

    // Capture the error with Sentry
    const eventId = Sentry.captureException(error, {
      contexts: {
        react: {
          componentStack: errorInfo.componentStack,
          errorBoundary: this.constructor.name,
        },
      },
    })

    this.setState({ eventId })

    // Call custom error handler if provided
    if (this.props.onError) {
      try {
        this.props.onError(error, errorInfo)
      } catch (handlerError) {
        console.error(
          '[ErrorBoundary] Error in custom error handler:',
          handlerError
        )
        Sentry.captureException(handlerError)
      }
    }

    // Log error to console in development
    if (import.meta.env.DEV) {
      console.group('[ErrorBoundary] React Error Caught')
      console.error('Error:', error)
      console.error('Error Info:', errorInfo)
      console.error('Component Stack:', errorInfo.componentStack)
      console.error('Sentry Event ID:', eventId)
      console.groupEnd()
    }
  }

  handleRetry = () => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
      eventId: null,
    })
  }

  handleReportFeedback = () => {
    if (this.state.eventId) {
      Sentry.showReportDialog({
        eventId: this.state.eventId,
        title: 'Help us improve',
        subtitle: 'Please describe what happened before the error occurred',
        subtitle2: '',
        labelComments: 'What happened?',
        labelClose: 'Cancel',
        labelSubmit: 'Submit',
      })
    }
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
              We&apos;ve encountered an unexpected error. Our team has been
              notified and we&apos;re looking into it.
            </p>

            <div className="flex items-center justify-center gap-3">
              <button
                onClick={this.handleRetry}
                className="px-6 py-2.5 bg-destructive text-destructive-foreground rounded-md hover:bg-destructive/90 focus:outline-none focus:ring-2 focus:ring-destructive focus:ring-offset-2 transition-colors font-medium"
              >
                Try Again
              </button>

              {this.props.showDialog && this.state.eventId && (
                <button
                  onClick={this.handleReportFeedback}
                  className="px-6 py-2.5 bg-background text-destructive border border-destructive/40 rounded-md hover:bg-destructive/10 focus:outline-none focus:ring-2 focus:ring-destructive focus:ring-offset-2 transition-colors font-medium"
                >
                  Report Issue
                </button>
              )}
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

                  {this.state.eventId && (
                    <div className="mt-4 text-xs text-muted-foreground">
                      Sentry Event ID: {this.state.eventId}
                    </div>
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
