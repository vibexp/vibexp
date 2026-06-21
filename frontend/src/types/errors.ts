// RFC 9457 Problem Details for HTTP APIs

export interface ValidationError {
  field: string
  message: string
  code: string
  constraint?: string
}

export interface APIErrorResponse {
  type: string
  title: string
  status: number
  detail: string
  code: string
  request_id: string
  timestamp: string
  instance?: string
  validation_errors?: ValidationError[]
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

    // Maintains proper stack trace for where our error was thrown (only available on V8)
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    if (Error.captureStackTrace) {
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
