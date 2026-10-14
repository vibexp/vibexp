import {
  accountErrorMessage,
  canDisconnect,
  canRemovePassword,
  isReauthenticationRequired,
} from '../accountRules'
import { account, apiError, identity, withPassword } from './fixtures'

describe('account rules', () => {
  it('keeps the only sign-in method', () => {
    const onlyGoogle = account()
    expect(canDisconnect(onlyGoogle, onlyGoogle.identities[0])).toBe(false)

    const onlyPassword = withPassword({ identities: [] })
    expect(canRemovePassword(onlyPassword)).toBe(false)
  })

  it('lets a method go while another usable one remains', () => {
    const both = withPassword()
    expect(canRemovePassword(both)).toBe(true)
    expect(canDisconnect(both, both.identities[0])).toBe(true)

    const github = identity({ id: 'gh', provider: 'github' })
    const two = account({ identities: [identity(), github] })
    expect(canDisconnect(two, github)).toBe(true)
  })

  it('does not count a disabled provider or a password that cannot be used', () => {
    const disabledProvider = withPassword({
      identities: [identity({ available: false })],
    })
    expect(canRemovePassword(disabledProvider)).toBe(false)

    // Password sign-in is off: the password is not a way in.
    const passwordOff = account({
      password: {
        set: true,
        changed_at: null,
        login_enabled: false,
        disabled: false,
      },
    })
    expect(canDisconnect(passwordOff, passwordOff.identities[0])).toBe(false)

    // A disabled identity can go while the password works.
    const stale = identity({ available: false })
    expect(canDisconnect(withPassword({ identities: [stale] }), stale)).toBe(
      true
    )
  })

  it.each([
    ['reauthentication_required', 'recent sign-in'],
    ['cookie_session_required', 'browser session'],
    ['password_login_disabled', 'switched off'],
    ['last_sign_in_method', 'only way left'],
    ['too_many_attempts', 'Too many wrong passwords'],
  ])('explains %s', (code, text) => {
    expect(
      accountErrorMessage(apiError(409, code, 'server text'), 'fallback')
    ).toContain(text)
  })

  it('shows the server message for any other error, or the fallback', () => {
    expect(
      accountErrorMessage(apiError(500, 'INTERNAL', 'It broke'), 'fallback')
    ).toBe('It broke')
    expect(accountErrorMessage('nope', 'fallback')).toBe('fallback')
  })

  it('recognises the recent sign-in refusal', () => {
    expect(
      isReauthenticationRequired(
        apiError(403, 'reauthentication_required', 'Sign in again')
      )
    ).toBe(true)
    expect(isReauthenticationRequired(new Error('x'))).toBe(false)
  })
})
