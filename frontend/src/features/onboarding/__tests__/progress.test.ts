import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { STORAGE_KEYS } from '@/constants/storageKeys'

import {
  clampStep,
  countDone,
  EMPTY_PROGRESS,
  isAllDone,
  type OnboardingProgress,
  readProgress,
  resetProgressCache,
  resumeStep,
  withStepStatus,
  writeProgress,
} from '../progress'

const KEY = STORAGE_KEYS.ONBOARDING_PROGRESS

function progress(
  status: OnboardingProgress['status'],
  lastStep = 0
): OnboardingProgress {
  return { status, lastStep }
}

beforeEach(() => {
  window.localStorage.clear()
  resetProgressCache()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('readProgress / writeProgress', () => {
  it('starts every step as todo when storage is empty', () => {
    expect(readProgress('u1')).toEqual(EMPTY_PROGRESS)
  })

  it('keeps each user apart under one key', () => {
    writeProgress(
      'u1',
      progress(['done', 'todo', 'todo', 'todo', 'todo', 'todo'], 1)
    )
    writeProgress(
      'u2',
      progress(['skipped', 'todo', 'todo', 'todo', 'todo', 'todo'])
    )
    resetProgressCache()

    expect(readProgress('u1').status[0]).toBe('done')
    expect(readProgress('u1').lastStep).toBe(1)
    expect(readProgress('u2').status[0]).toBe('skipped')
    const stored = JSON.parse(
      window.localStorage.getItem(KEY) ?? '{}'
    ) as Record<string, unknown>
    expect(Object.keys(stored).sort()).toEqual(['u1', 'u2'])
  })

  it('reads malformed storage as no progress', () => {
    window.localStorage.setItem(KEY, '{not json')
    expect(readProgress('u1')).toEqual(EMPTY_PROGRESS)

    resetProgressCache()
    window.localStorage.setItem(
      KEY,
      JSON.stringify({ u1: { status: ['done', 'bogus', 7], lastStep: 99 } })
    )
    expect(readProgress('u1')).toEqual({
      ...progress(['done', 'todo', 'todo', 'todo', 'todo', 'todo'], 5),
      cardDismissed: false,
    })
  })

  it('still works for the visit when storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })
    vi.spyOn(console, 'error').mockImplementation(() => undefined)

    expect(readProgress('u1')).toEqual(EMPTY_PROGRESS)
    const next = progress(['done', 'done', 'todo', 'todo', 'todo', 'todo'], 2)
    expect(() => {
      writeProgress('u1', next)
    }).not.toThrow()
    // Served from memory for the rest of the visit.
    expect(readProgress('u1')).toEqual(next)
  })
})

describe('step helpers', () => {
  it('marks a step, but a done step stays done when skipped later', () => {
    const done = withStepStatus(EMPTY_PROGRESS, 2, 'done')
    expect(done.status[2]).toBe('done')
    expect(withStepStatus(done, 2, 'skipped').status[2]).toBe('done')
    expect(withStepStatus(EMPTY_PROGRESS, 2, 'skipped').status[2]).toBe(
      'skipped'
    )
    // Never mutates.
    expect(EMPTY_PROGRESS.status[2]).toBe('todo')
  })

  it('counts done steps out of five, ignoring the celebration', () => {
    const p = progress(['done', 'skipped', 'done', 'todo', 'todo', 'done'])
    expect(countDone(p)).toBe(2)
    expect(isAllDone(p)).toBe(false)
    expect(
      isAllDone(progress(['done', 'done', 'done', 'done', 'done', 'todo']))
    ).toBe(true)
  })

  it('resumes at the first step not done, and replays from the start when all are', () => {
    expect(
      resumeStep(progress(['done', 'done', 'skipped', 'todo', 'todo', 'todo']))
    ).toBe(2)
    expect(resumeStep(EMPTY_PROGRESS)).toBe(0)
    expect(
      resumeStep(progress(['done', 'done', 'done', 'done', 'done', 'todo']))
    ).toBe(0)
  })

  it('clamps a step to the flow', () => {
    expect(clampStep(-3)).toBe(0)
    expect(clampStep(42)).toBe(5)
    expect(clampStep(Number('abc'))).toBe(0)
    expect(clampStep(2.7)).toBe(2)
  })
})
