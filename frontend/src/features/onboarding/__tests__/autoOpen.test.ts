import { describe, expect, it } from 'vitest'

import {
  type AutoOpenInput,
  gettingStartedHref,
  shouldAutoOpenOnboarding,
} from '../autoOpen'

const base: AutoOpenInput = {
  user: { id: 'u1', onboarding_completed: false },
  pathname: '/',
  sessionFlag: null,
  pendingInvite: false,
  pendingReturnTo: false,
}

describe('shouldAutoOpenOnboarding', () => {
  it('opens for a user who has not finished it, landing on the dashboard', () => {
    expect(shouldAutoOpenOnboarding(base)).toBe(true)
  })

  it('does not open once the user has completed onboarding', () => {
    expect(
      shouldAutoOpenOnboarding({
        ...base,
        user: { id: 'u1', onboarding_completed: true },
      })
    ).toBe(false)
  })

  it('does not open without a signed-in user', () => {
    expect(shouldAutoOpenOnboarding({ ...base, user: null })).toBe(false)
  })

  it.each(['/memories/abc', '/feeds/items/1', '/getting-started', '/settings'])(
    'never hijacks a deep link (%s)',
    pathname => {
      expect(shouldAutoOpenOnboarding({ ...base, pathname })).toBe(false)
    }
  )

  it('opens at most once per browser session for the same user', () => {
    expect(shouldAutoOpenOnboarding({ ...base, sessionFlag: 'u1' })).toBe(false)
  })

  it('still opens for another user who signs in on the same session', () => {
    expect(shouldAutoOpenOnboarding({ ...base, sessionFlag: 'u2' })).toBe(true)
  })

  it('stands aside for a pending invitation', () => {
    expect(shouldAutoOpenOnboarding({ ...base, pendingInvite: true })).toBe(
      false
    )
  })

  it('stands aside for a pending return_to', () => {
    expect(shouldAutoOpenOnboarding({ ...base, pendingReturnTo: true })).toBe(
      false
    )
  })
})

describe('gettingStartedHref', () => {
  it('deep-links a step', () => {
    expect(gettingStartedHref(3)).toBe('/getting-started?step=3')
  })
})
