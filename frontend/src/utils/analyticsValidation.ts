/**
 * Analytics Event Validation Utilities
 *
 * This module provides validation functions for analytics events to ensure
 * data quality and consistency. It helps catch invalid events before they
 * are sent to GTM and provides helpful error messages for debugging.
 *
 * Features:
 * - Event structure validation
 * - Parameter type checking
 * - Required field validation
 * - Development warnings for invalid events
 * - GTM compatibility checks
 */

import type {
  AnalyticsEvent,
  TrackAuthParams,
  TrackEventParams,
  TrackPageParams,
  UserProperties,
} from '../types/analytics'
import { ANALYTICS_EVENTS } from '../types/analytics'

export interface ValidationResult {
  isValid: boolean
  errors: string[]
  warnings: string[]
}

/**
 * Validate user properties structure
 */
export function validateUserProperties(
  userProperties: UserProperties
): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  // Required fields
  if (!userProperties.user_id || typeof userProperties.user_id !== 'string') {
    errors.push('user_id is required and must be a string')
  }

  if (!userProperties.email || typeof userProperties.email !== 'string') {
    errors.push('email is required and must be a string')
  }

  if (!userProperties.name || typeof userProperties.name !== 'string') {
    errors.push('name is required and must be a string')
  }

  // Optional field validation
  if (
    userProperties.signup_date &&
    typeof userProperties.signup_date !== 'string'
  ) {
    warnings.push('signup_date should be a string (ISO date)')
  }

  if (
    userProperties.avatar_url &&
    typeof userProperties.avatar_url !== 'string'
  ) {
    warnings.push('avatar_url should be a string')
  }

  if (
    userProperties.created_at &&
    typeof userProperties.created_at !== 'string'
  ) {
    warnings.push('created_at should be a string (ISO date)')
  }

  // Email format validation
  if (userProperties.email && !isValidEmail(userProperties.email)) {
    warnings.push('email format appears invalid')
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Validate base event structure
 */
export function validateBaseEvent(event: AnalyticsEvent): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  // Required fields
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  if (!event.event || typeof event.event !== 'string') {
    errors.push('event name is required and must be a string')
  }

  if (!event.timestamp || typeof event.timestamp !== 'number') {
    errors.push('timestamp is required and must be a number')
  }

  if (!event.page_path || typeof event.page_path !== 'string') {
    errors.push('page_path is required and must be a string')
  }

  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  if (!event.environment || typeof event.environment !== 'string') {
    errors.push('environment is required and must be a string')
  }

  // Validate event name against known events
  const knownEvents = Object.values(ANALYTICS_EVENTS)
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition, @typescript-eslint/no-explicit-any, @typescript-eslint/no-unsafe-argument
  if (event.event && !knownEvents.includes(event.event as any)) {
    warnings.push(
      `Unknown event name: ${event.event}. Consider adding it to ANALYTICS_EVENTS`
    )
  }

  // Validate timestamp is reasonable (not too far in past/future)
  if (event.timestamp) {
    const now = Date.now()
    const timeDiff = Math.abs(now - event.timestamp)
    const oneHour = 60 * 60 * 1000

    if (timeDiff > oneHour) {
      warnings.push('timestamp is more than 1 hour different from current time')
    }
  }

  // Validate user properties if present
  if (event.user_properties) {
    const userPropsValidation = validateUserProperties(event.user_properties)
    errors.push(...userPropsValidation.errors)
    warnings.push(...userPropsValidation.warnings)
  }

  // Validate page path format
  if (event.page_path && !event.page_path.startsWith('/')) {
    warnings.push('page_path should start with "/"')
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Validate track event parameters
 */
export function validateTrackEventParams(
  params: TrackEventParams
): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  if (!params.event || typeof params.event !== 'string') {
    errors.push('event name is required and must be a string')
  }

  if (params.userProperties) {
    const userPropsValidation = validateUserProperties(params.userProperties)
    errors.push(...userPropsValidation.errors)
    warnings.push(...userPropsValidation.warnings)
  }

  if (params.properties && typeof params.properties !== 'object') {
    errors.push('properties must be an object')
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Validate track page parameters
 */
export function validateTrackPageParams(
  params: TrackPageParams
): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  if (!params.path || typeof params.path !== 'string') {
    errors.push('path is required and must be a string')
  }

  if (!params.title || typeof params.title !== 'string') {
    errors.push('title is required and must be a string')
  }

  if (params.path && !params.path.startsWith('/')) {
    warnings.push('path should start with "/"')
  }

  if (params.referrer && typeof params.referrer !== 'string') {
    warnings.push('referrer should be a string')
  }

  if (params.userProperties) {
    const userPropsValidation = validateUserProperties(params.userProperties)
    errors.push(...userPropsValidation.errors)
    warnings.push(...userPropsValidation.warnings)
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Validate track auth parameters
 */
export function validateTrackAuthParams(
  params: TrackAuthParams
): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  const validEventTypes = ['signin_page_view', 'signed_in', 'logged_out']
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
  if (!params.eventType || !validEventTypes.includes(params.eventType)) {
    errors.push(`eventType must be one of: ${validEventTypes.join(', ')}`)
  }

  if (params.userProperties) {
    const userPropsValidation = validateUserProperties(params.userProperties)
    errors.push(...userPropsValidation.errors)
    warnings.push(...userPropsValidation.warnings)
  }

  // For signed_in events, user properties should be present
  if (params.eventType === 'signed_in' && !params.userProperties) {
    warnings.push('signed_in events should include user properties')
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Validate GTM compatibility
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function validateGTMCompatibility(eventData: any): ValidationResult {
  const errors: string[] = []
  const warnings: string[] = []

  // Check for reserved GTM properties
  const reservedProperties = [
    'gtm.start',
    'gtm.uniqueEventId',
    'gtm.element',
    'gtm.elementClasses',
  ]
  // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
  Object.keys(eventData).forEach(key => {
    if (reservedProperties.includes(key)) {
      warnings.push(
        `Property "${key}" is reserved by GTM and may be overwritten`
      )
    }
  })

  // Check for property name length (GTM has limits)
  // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
  Object.keys(eventData).forEach(key => {
    if (key.length > 100) {
      warnings.push(
        `Property name "${key}" is very long and may be truncated by GTM`
      )
    }
  })

  // Check for deep nesting (GTM has limits)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const checkDepth = (obj: any, depth = 0): number => {
    if (depth > 10) return depth
    if (typeof obj !== 'object' || obj === null) return depth

    let maxDepth = depth
    // eslint-disable-next-line @typescript-eslint/no-unsafe-argument
    Object.values(obj).forEach(value => {
      const nestedDepth = checkDepth(value, depth + 1)
      maxDepth = Math.max(maxDepth, nestedDepth)
    })
    return maxDepth
  }

  const depth = checkDepth(eventData)
  if (depth > 5) {
    warnings.push(
      'Event data is deeply nested and may not be fully processed by GTM'
    )
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  }
}

/**
 * Helper function to validate email format
 */
function isValidEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  return emailRegex.test(email)
}

/**
 * Comprehensive validation for any analytics event
 */
export function validateAnalyticsEvent(
  event: AnalyticsEvent
): ValidationResult {
  const baseValidation = validateBaseEvent(event)
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const gtmValidation = validateGTMCompatibility(event as any)

  return {
    isValid: baseValidation.isValid && gtmValidation.isValid,
    errors: [...baseValidation.errors, ...gtmValidation.errors],
    warnings: [...baseValidation.warnings, ...gtmValidation.warnings],
  }
}

/**
 * Log validation results to console (development only)
 */
export function logValidationResults(
  eventName: string,
  validation: ValidationResult,
  enableLogging = process.env.NODE_ENV === 'development' ||
    process.env.NODE_ENV === 'test'
): void {
  if (!enableLogging) return

  if (!validation.isValid) {
    console.group(`[Analytics Validation] ❌ ${eventName} - INVALID`)
    validation.errors.forEach(error => {
      console.error('Error:', error)
    })
    validation.warnings.forEach(warning => {
      console.warn('Warning:', warning)
    })
    console.groupEnd()
  } else if (validation.warnings.length > 0) {
    console.group(
      `[Analytics Validation] ⚠️ ${eventName} - Valid with warnings`
    )
    validation.warnings.forEach(warning => {
      console.warn('Warning:', warning)
    })
    console.groupEnd()
  } else {
    console.log(`[Analytics Validation] ✅ ${eventName} - Valid`)
  }
}
