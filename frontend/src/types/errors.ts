// RFC 9457 Problem Details for HTTP APIs

import type { components } from '@vibexp/api-client'

export type ValidationError = components['schemas']['ValidationError']

// The backend sends `metadata` on some errors (internal/errors/response.go,
// e.g. RESOURCE_LIMIT_EXCEEDED details read by utils/apiErrorMeta.ts) but the
// OpenAPI ErrorResponse schema doesn't document it yet — keep it as a local
// extension until the spec gap is closed (see issue #90 discussion / #89).
export type APIErrorResponse = components['schemas']['ErrorResponse'] & {
  metadata?: Record<string, unknown>
}

export class ApiError extends Error {
  public readonly response: APIErrorResponse
  public readonly status: number
  public readonly code: string
  public readonly requestId: string
  public readonly validationErrors?: ValidationError[]
  public readonly metadata?: Record<string, unknown>

  constructor(response: APIErrorResponse) {
    super(response.detail || response.title)
    this.name = 'ApiError'
    this.response = response
    this.status = response.status
    this.code = response.code
    this.requestId = response.request_id
    this.validationErrors = response.validation_errors
    this.metadata = response.metadata

    // Maintains proper stack trace for where our error was thrown. Only V8
    // provides captureStackTrace; the type declares it always-present, so
    // guard with the `in` operator to keep the runtime check lint-clean.
    if ('captureStackTrace' in Error) {
      Error.captureStackTrace(this, ApiError)
    }
  }

  /**
   * Get human-readable error message
   */
  getMessage(): string {
    if (this.validationErrors && this.validationErrors.length > 0) {
      return this.validationErrors.map(ve => ve.message).join(', ')
    }
    return this.response.detail || this.response.title
  }

  /**
   * Get validation errors for a specific field
   */
  getFieldErrors(fieldName: string): ValidationError[] {
    if (!this.validationErrors) return []
    return this.validationErrors.filter(ve => ve.field === fieldName)
  }

  /**
   * Check if this is a validation error
   */
  isValidationError(): boolean {
    return this.code === 'VALIDATION_FAILED'
  }

  /**
   * Check if this is an authentication error
   */
  isAuthError(): boolean {
    return (
      this.code === 'AUTH_REQUIRED' ||
      this.code === 'AUTH_INVALID' ||
      this.code === 'AUTH_EXPIRED'
    )
  }

  /**
   * Check if this is a not found error
   */
  isNotFoundError(): boolean {
    return this.code === 'RESOURCE_NOT_FOUND'
  }

  /**
   * Check if this is a resource limit exceeded error
   */
  isResourceLimitError(): boolean {
    return this.code === 'RESOURCE_LIMIT_EXCEEDED'
  }
}
