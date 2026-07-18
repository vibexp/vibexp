import { useCallback } from 'react'

import { useAlertContext } from '../contexts/AlertContext'
import type { AlertType } from '../types/alert'
import { ApiError, type ValidationError } from '../types/errors'
import { storageUtils } from '../utils/storage'

/**
 * Handle ApiError instances
 */
function handleApiError(
  error: ApiError,
  shouldShowToast: boolean,
  showAlert: (options: { message: string; type: AlertType }) => void
): Record<string, string> {
  const notify = (message: string, type: AlertType) => {
    if (shouldShowToast) {
      showAlert({ message, type })
    }
  }

  // Handle authentication errors
  if (error.isAuthError()) {
    notify('Authentication required. Please log in again.', 'error')
    // Clear all VibeXP storage and redirect to login
    storageUtils.clearVibeXPData()
    window.location.href = '/login'
    return {}
  }

  // Handle validation errors with field details
  if (error.isValidationError() && error.validationErrors) {
    const fieldErrors = extractFieldErrors(error)
    const errorCount = Object.keys(fieldErrors).length

    notify(
      errorCount === 1
        ? 'Please fix the validation error'
        : `Please fix ${String(errorCount)} validation errors`,
      'error'
    )

    return fieldErrors
  }

  // Handle resource limit errors
  if (error.isResourceLimitError()) {
    notify(error.getMessage(), 'warning')
    return {}
  }

  // Handle permission errors. The UI gates actions the caller's role lacks
  // (#225), but the backend is the enforcement point, so a 403 can still land
  // here — a stale permission set after a role change, or a direct link. Say
  // why it failed instead of dropping it into the generic error toast.
  //
  // Deliberately below the auth branch: a forbidden caller is authenticated,
  // so this must never trigger the logout redirect above.
  if (error.isForbidden()) {
    notify(error.getMessage(), 'warning')
    return {}
  }

  // Show general error message
  notify(error.getMessage(), 'error')
  return {}
}

/**
 * Handle standard Error instances
 */
function handleStandardError(
  error: Error,
  shouldShowToast: boolean,
  showAlert: (options: { message: string; type: AlertType }) => void
): Record<string, string> {
  if (error.message.includes('timeout')) {
    if (shouldShowToast) {
      showAlert({
        message: 'Request timed out. Please try again.',
        type: 'error',
      })
    }
    return {}
  }

  if (error.message.includes('Network error')) {
    if (shouldShowToast) {
      showAlert({
        message: 'Network error. Please check your connection.',
        type: 'error',
      })
    }
    return {}
  }

  if (shouldShowToast) {
    showAlert({
      message: error.message,
      type: 'error',
    })
  }
  return {}
}

interface UseErrorHandlerOptions {
  showToast?: boolean
}

interface UseErrorHandlerReturn {
  handleError: (
    error: unknown,
    defaultMessage?: string,
    options?: UseErrorHandlerOptions
  ) => Record<string, string>
  getFieldError: (error: unknown, fieldName: string) => string | null
  isFieldError: (error: unknown, fieldName: string) => boolean
}

/**
 * Hook for consistent error handling across the application
 *
 * Features:
 * - Auto-displays error toasts
 * - Extracts field-level validation errors
 * - Handles auth errors with redirect
 * - Provides helper methods for field-specific errors
 *
 * @example
 * ```tsx
 * const { handleError, getFieldError } = useErrorHandler();
 *
 * try {
 *   await createAgent(data);
 * } catch (error) {
 *   handleError(error);
 * }
 *
 * // Get field-specific error
 * const nameError = getFieldError(error, 'name');
 * ```
 */
export function useErrorHandler(): UseErrorHandlerReturn {
  const { showAlert } = useAlertContext()

  /**
   * Handle error and display appropriate message
   * Returns field errors map for validation errors
   */
  const handleError = useCallback(
    (
      error: unknown,
      defaultMessage?: string,
      options?: UseErrorHandlerOptions
    ): Record<string, string> => {
      const shouldShowToast = options?.showToast ?? true

      if (error instanceof ApiError) {
        return handleApiError(error, shouldShowToast, showAlert)
      }

      if (error instanceof Error) {
        return handleStandardError(error, shouldShowToast, showAlert)
      }

      // Fallback message
      if (shouldShowToast) {
        showAlert({
          message: defaultMessage ?? 'An unexpected error occurred',
          type: 'error',
        })
      }
      return {}
    },
    [showAlert]
  )

  /**
   * Get validation error message for a specific field
   */
  const getFieldError = useCallback(
    (error: unknown, fieldName: string): string | null => {
      if (error instanceof ApiError && error.validationErrors) {
        const fieldErrors = error.getFieldErrors(fieldName)
        if (fieldErrors.length > 0) {
          return fieldErrors[0].message
        }
      }
      return null
    },
    []
  )

  /**
   * Check if a specific field has validation errors
   */
  const isFieldError = useCallback(
    (error: unknown, fieldName: string): boolean => {
      if (error instanceof ApiError && error.validationErrors) {
        return error.getFieldErrors(fieldName).length > 0
      }
      return false
    },
    []
  )

  return { handleError, getFieldError, isFieldError }
}

/**
 * Extract all field errors from an ApiError
 */
export function extractFieldErrors(error: unknown): Record<string, string> {
  const fieldErrors: Record<string, string> = {}

  if (error instanceof ApiError && error.validationErrors) {
    error.validationErrors.forEach((ve: ValidationError) => {
      if (!fieldErrors[ve.field]) {
        fieldErrors[ve.field] = ve.message
      }
    })
  }

  return fieldErrors
}

/**
 * Check if error is a specific error code
 */
export function isErrorCode(error: unknown, code: string): boolean {
  return error instanceof ApiError && error.code === code
}

/**
 * Get request ID from error for support tracking
 */
export function getErrorRequestId(error: unknown): string | null {
  if (error instanceof ApiError) {
    return error.requestId || null
  }
  return null
}
