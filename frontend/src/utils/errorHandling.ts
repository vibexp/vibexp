import { ApiError } from '../types/errors'

/**
 * Extract human-readable error message from any error type.
 * Prioritizes ApiError messages from backend over generic Error messages.
 *
 * @param error - The error object (ApiError, Error, or unknown)
 * @param fallbackMessage - Message to use if no better message is available
 * @returns Human-readable error message
 *
 * @example
 * ```typescript
 * try {
 *   await someApiCall()
 * } catch (error) {
 *   const message = getErrorMessage(error, 'Operation failed')
 *   showAlert({ message, type: 'error' })
 * }
 * ```
 */
export function getErrorMessage(
  error: unknown,
  fallbackMessage = 'An unexpected error occurred'
): string {
  // Priority 1: ApiError with backend message
  if (error instanceof ApiError) {
    return error.getMessage()
  }

  // Priority 2: Standard Error with message
  if (error instanceof Error) {
    return error.message
  }

  // Priority 3: Fallback message
  return fallbackMessage
}

/**
 * Get error code from ApiError if available
 *
 * @param error - The error object
 * @returns Error code string or null if not an ApiError
 *
 * @example
 * ```typescript
 * const code = getErrorCode(error)
 * if (code === 'VALIDATION_FAILED') {
 *   // Handle validation errors
 * }
 * ```
 */
export function getErrorCode(error: unknown): string | null {
  if (error instanceof ApiError) {
    return error.code
  }
  return null
}

/**
 * Check if error is a specific ApiError code
 *
 * @param error - The error object
 * @param code - The error code to check for
 * @returns True if error is an ApiError with the specified code
 *
 * @example
 * ```typescript
 * if (isErrorCode(error, 'RESOURCE_NOT_FOUND')) {
 *   // Handle missing resource
 * }
 * ```
 */
export function isErrorCode(error: unknown, code: string): boolean {
  return error instanceof ApiError && error.code === code
}
