import { renderHook } from '@testing-library/react'
import { ReactNode } from 'react'

import { AlertProvider } from '../../src/contexts/AlertContext'
import { useErrorHandler } from '../../src/hooks/useErrorHandler'
import { ApiError } from '../../src/types/errors'

// Wrapper with AlertProvider
const wrapper = ({ children }: { children: ReactNode }) => (
  <AlertProvider>{children}</AlertProvider>
)

describe('useErrorHandler Integration with API Errors', () => {
  it('returns field errors map for validation errors', () => {
    const { result } = renderHook(() => useErrorHandler(), { wrapper })

    // Simulate backend validation error response
    const apiErrorResponse = {
      type: 'https://api.vibexp.io/errors/validation',
      title: 'Validation Failed',
      status: 400,
      detail: 'Request validation failed',
      code: 'VALIDATION_FAILED',
      request_id: 'req-123',
      timestamp: '2024-01-01T00:00:00Z',
      validation_errors: [
        {
          field: 'name',
          message: 'Name is required',
          code: 'required',
        },
        {
          field: 'base_url',
          message: 'Invalid URL format',
          code: 'invalid_url',
        },
      ],
    }

    const apiError = new ApiError(apiErrorResponse)
    const fieldErrors = result.current.handleError(apiError)

    expect(fieldErrors).toEqual({
      name: 'Name is required',
      base_url: 'Invalid URL format',
    })
  })

  it('returns empty object for non-validation errors', () => {
    const { result } = renderHook(() => useErrorHandler(), { wrapper })

    const apiErrorResponse = {
      type: 'https://api.vibexp.io/errors/not-found',
      title: 'Not Found',
      status: 404,
      detail: 'Resource not found',
      code: 'RESOURCE_NOT_FOUND',
      request_id: 'req-123',
      timestamp: '2024-01-01T00:00:00Z',
    }

    const apiError = new ApiError(apiErrorResponse)
    const fieldErrors = result.current.handleError(apiError)

    expect(fieldErrors).toEqual({})
  })

  it('returns empty object for standard errors', () => {
    const { result } = renderHook(() => useErrorHandler(), { wrapper })

    const standardError = new Error('Network error occurred')
    const fieldErrors = result.current.handleError(standardError)

    expect(fieldErrors).toEqual({})
  })

  it('extracts field errors from ApiError validation response', () => {
    // Simulate backend validation error response
    const apiErrorResponse = {
      type: 'https://api.vibexp.io/errors/validation',
      title: 'Validation Failed',
      status: 400,
      detail: 'Request validation failed',
      code: 'VALIDATION_FAILED',
      request_id: 'req-123',
      timestamp: '2024-01-01T00:00:00Z',
      validation_errors: [
        {
          field: 'name',
          message: 'Name is required',
          code: 'required',
        },
        {
          field: 'base_url',
          message: 'Invalid URL format',
          code: 'invalid_url',
        },
      ],
    }

    const apiError = new ApiError(apiErrorResponse)

    // Verify ApiError properly stores validation errors
    expect(apiError.isValidationError()).toBe(true)
    expect(apiError.validationErrors).toHaveLength(2)
    expect(apiError.validationErrors?.[0].field).toBe('name')
    expect(apiError.validationErrors?.[1].field).toBe('base_url')
  })

  it('getFieldErrors returns errors for specific field', () => {
    const apiErrorResponse = {
      type: 'https://api.vibexp.io/errors/validation',
      title: 'Validation Failed',
      status: 400,
      detail: 'Request validation failed',
      code: 'VALIDATION_FAILED',
      request_id: 'req-123',
      timestamp: '2024-01-01T00:00:00Z',
      validation_errors: [
        {
          field: 'name',
          message: 'Name is required',
          code: 'required',
        },
        {
          field: 'name',
          message: 'Name must be unique',
          code: 'unique',
        },
        {
          field: 'base_url',
          message: 'Invalid URL format',
          code: 'invalid_url',
        },
      ],
    }

    const apiError = new ApiError(apiErrorResponse)

    const nameErrors = apiError.getFieldErrors('name')
    expect(nameErrors).toHaveLength(2)
    expect(nameErrors[0].message).toBe('Name is required')
    expect(nameErrors[1].message).toBe('Name must be unique')

    const urlErrors = apiError.getFieldErrors('base_url')
    expect(urlErrors).toHaveLength(1)
    expect(urlErrors[0].message).toBe('Invalid URL format')

    const nonExistentErrors = apiError.getFieldErrors('non_existent')
    expect(nonExistentErrors).toHaveLength(0)
  })

  it('suppresses toast when showToast option is false', () => {
    const { result } = renderHook(() => useErrorHandler(), { wrapper })

    const apiErrorResponse = {
      type: 'https://api.vibexp.io/errors/validation',
      title: 'Validation Failed',
      status: 400,
      detail: 'Request validation failed',
      code: 'VALIDATION_FAILED',
      request_id: 'req-123',
      timestamp: '2024-01-01T00:00:00Z',
      validation_errors: [
        {
          field: 'name',
          message: 'Name is required',
          code: 'required',
        },
      ],
    }

    const apiError = new ApiError(apiErrorResponse)
    const fieldErrors = result.current.handleError(apiError, undefined, {
      showToast: false,
    })

    // Still returns field errors even with toast suppressed
    expect(fieldErrors).toEqual({
      name: 'Name is required',
    })
  })
})
