/**
 * @jest-environment jsdom
 *
 * Tests for the sentryBeforeSend filter logic.
 *
 * sentryBeforeSend lives in src/utils/sentry-filters.ts — a Vite-independent
 * module with no import.meta.env or React Router dependencies — so it can be
 * imported directly in Jest's CommonJS environment without any module mocking.
 */

import {
  sentryBeforeSend,
  type SentryFilterEvent,
  type SentryFilterHint,
} from '../../src/utils/sentry-filters'

function makeHint(error?: unknown): SentryFilterHint {
  return { originalException: error }
}

const DUMMY_EVENT: SentryFilterEvent = { event_id: 'test-event-id' }

describe('sentryBeforeSend', () => {
  describe('ResizeObserver errors (existing behaviour)', () => {
    it('returns null for a ResizeObserver error', () => {
      const error = new Error('ResizeObserver loop limit exceeded')
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBeNull()
    })

    it('returns null for a ResizeObserver loop completed notification', () => {
      const error = new Error(
        'ResizeObserver loop completed with undelivered notifications.'
      )
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBeNull()
    })
  })

  describe('NotFoundError — removeChild (VIBEXP-FRONTEND-JS-7)', () => {
    it('returns null for the exact removeChild message', () => {
      const error = new Error(
        "Failed to execute 'removeChild' on 'Node': The node to be removed is not a child of this node."
      )
      error.name = 'NotFoundError'
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBeNull()
    })

    it('returns null when the message only contains "removeChild"', () => {
      const error = new Error('removeChild called on detached node')
      error.name = 'NotFoundError'
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBeNull()
    })
  })

  describe('NotFoundError — object not found (VIBEXP-FRONTEND-JS-8)', () => {
    it('returns null for the exact "The object can not be found here" message', () => {
      const error = new Error('The object can not be found here.')
      error.name = 'NotFoundError'
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBeNull()
    })
  })

  describe('does NOT filter unrelated errors', () => {
    it('passes through a regular TypeError', () => {
      const error = new TypeError('Cannot read properties of undefined')
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBe(DUMMY_EVENT)
    })

    it('passes through a NotFoundError with an unrecognised message', () => {
      const error = new Error('Some other not-found condition')
      error.name = 'NotFoundError'
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBe(DUMMY_EVENT)
    })

    it('passes through when the hint has no originalException', () => {
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(undefined))
      expect(result).toBe(DUMMY_EVENT)
    })

    it('passes through when originalException is a string (not an Error instance)', () => {
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint('removeChild'))
      expect(result).toBe(DUMMY_EVENT)
    })

    it('passes through a network error', () => {
      const error = new Error('Failed to fetch')
      const result = sentryBeforeSend(DUMMY_EVENT, makeHint(error))
      expect(result).toBe(DUMMY_EVENT)
    })
  })
})
