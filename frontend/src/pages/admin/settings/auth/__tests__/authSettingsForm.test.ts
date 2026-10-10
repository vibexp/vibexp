import {
  allowlistImpactSentence,
  availableProviderTypes,
  emptyProviderForm,
  isAllowlistDomain,
  isAllowlistEmail,
  issuerUrlError,
  lockoutRiskCopy,
  lockoutRiskReason,
  moveProvider,
  nextSortOrder,
  normalizeAllowlistEntries,
  redirectUriFor,
  sameAllowlistEntries,
  sortProviders,
  toProviderCreate,
  toProviderForm,
  toProviderTest,
  toProviderUpdate,
  validateAllowlistForm,
  validateProviderForm,
} from '../authSettingsForm'
import {
  apiError,
  github,
  google,
  lockoutRisk,
  provider,
  REDIRECT_URI,
  versionConflict,
} from './fixtures'

describe('availableProviderTypes', () => {
  it('offers every type when nothing is stored', () => {
    expect(availableProviderTypes([])).toEqual(['google', 'github', 'oidc'])
  })

  it('hides Google and GitHub once used, and never OIDC', () => {
    expect(availableProviderTypes([google, provider()])).toEqual([
      'github',
      'oidc',
    ])
    expect(availableProviderTypes([google, github, provider()])).toEqual([
      'oidc',
    ])
  })
})

describe('validateProviderForm', () => {
  const valid = {
    ...emptyProviderForm('oidc'),
    slug: 'okta',
    display_name: 'Okta',
    client_id: 'abc',
    client_secret: 's3cret',
    issuer_url: 'https://example.okta.com',
  }

  it('accepts a complete new OIDC provider', () => {
    expect(validateProviderForm(valid, null)).toEqual({})
  })

  it.each(['', '-okta', 'Okta', 'ok ta', 'a'.repeat(64)])(
    'rejects the slug %j on a new provider',
    slug => {
      expect(validateProviderForm({ ...valid, slug }, null).slug).toBeDefined()
    }
  )

  it('does not judge the slug of a stored provider, which cannot change', () => {
    const stored = provider()
    const form = { ...toProviderForm(stored), slug: 'Not A Slug' }
    expect(validateProviderForm(form, stored).slug).toBeUndefined()
  })

  it('requires a display name and a client id', () => {
    const errors = validateProviderForm(
      { ...valid, display_name: '  ', client_id: '' },
      null
    )
    expect(errors.display_name).toBeDefined()
    expect(errors.client_id).toBeDefined()
  })

  it('requires an issuer URL for OIDC only', () => {
    expect(
      validateProviderForm({ ...valid, issuer_url: '' }, null).issuer_url
    ).toBeDefined()
    const googleForm = { ...emptyProviderForm('google'), client_id: 'a' }
    expect(
      validateProviderForm({ ...googleForm, client_secret: 'x' }, null)
    ).toEqual({})
  })

  it('requires the client secret on a new provider', () => {
    expect(
      validateProviderForm({ ...valid, client_secret: '' }, null).client_secret
    ).toBe('Enter the client secret.')
  })

  it('keeps the stored secret on an edit that leaves it blank', () => {
    const stored = provider()
    expect(validateProviderForm(toProviderForm(stored), stored)).toEqual({})
  })

  it('requires the secret again when an edit changes the issuer URL', () => {
    const stored = provider()
    const form = {
      ...toProviderForm(stored),
      issuer_url: 'https://other.okta.com',
    }
    expect(validateProviderForm(form, stored).client_secret).toMatch(
      /not kept when the issuer URL changes/
    )
    expect(
      validateProviderForm({ ...form, client_secret: 'new' }, stored)
    ).toEqual({})
  })
})

describe('issuerUrlError', () => {
  it.each([
    'https://example.okta.com',
    'https://example.com/realms/main',
    'http://localhost:8081',
    'http://127.0.0.1:9000/issuer',
  ])('accepts %s', url => {
    expect(issuerUrlError(url)).toBeNull()
  })

  it.each([
    ['', /Enter the issuer URL/],
    ['example.okta.com', /absolute URL/],
    ['http://example.okta.com', /must use https/],
    ['https://user:pw@example.com', /credentials, a query or a fragment/],
    ['https://example.com?x=1', /credentials, a query or a fragment/],
    ['https://example.com#frag', /credentials, a query or a fragment/],
  ])('rejects %j', (url, message) => {
    expect(issuerUrlError(url)).toMatch(message)
  })
})

describe('wire mapping', () => {
  it('creates an OIDC provider with its issuer and the expected version', () => {
    const form = {
      ...emptyProviderForm('oidc'),
      slug: 'okta',
      display_name: ' Okta ',
      client_id: ' abc ',
      client_secret: 's3cret',
      issuer_url: ' https://example.okta.com ',
    }
    expect(toProviderCreate(form, 3, 7)).toEqual({
      type: 'oidc',
      slug: 'okta',
      display_name: 'Okta',
      enabled: true,
      sort_order: 3,
      client_id: 'abc',
      client_secret: 's3cret',
      issuer_url: 'https://example.okta.com',
      expected_version: 7,
    })
  })

  it('creates a Google provider with no issuer URL', () => {
    const form = {
      ...emptyProviderForm('google'),
      client_id: 'abc',
      client_secret: 'x',
    }
    expect(toProviderCreate(form, 0, 1)).not.toHaveProperty('issuer_url')
  })

  it('omits a blank client secret from an update, keeping the stored one', () => {
    const stored = provider()
    const body = toProviderUpdate(stored, toProviderForm(stored), 4, false)
    expect(body).toEqual({
      display_name: 'Okta',
      enabled: true,
      sort_order: 0,
      client_id: 'client-okta',
      issuer_url: 'https://example.okta.com',
      expected_version: 4,
      confirm_lockout_risk: false,
    })
  })

  it('sends a typed client secret and the lockout confirmation', () => {
    const stored = provider()
    const body = toProviderUpdate(
      stored,
      { ...toProviderForm(stored), client_secret: 'rotated', enabled: false },
      4,
      true
    )
    expect(body.client_secret).toBe('rotated')
    expect(body.enabled).toBe(false)
    expect(body.confirm_lockout_risk).toBe(true)
  })

  it('never sends an issuer URL for a Google provider', () => {
    expect(
      toProviderUpdate(google, { sort_order: 5 }, 2, false)
    ).not.toHaveProperty('issuer_url')
  })

  it('tests an unsaved provider as a full candidate', () => {
    const form = {
      ...emptyProviderForm('oidc'),
      slug: 'okta',
      client_id: 'abc',
      client_secret: 's3cret',
      issuer_url: 'https://example.okta.com',
    }
    expect(toProviderTest(form, null)).toEqual({
      type: 'oidc',
      client_id: 'abc',
      client_secret: 's3cret',
      issuer_url: 'https://example.okta.com',
    })
  })

  it('tests a stored provider by id plus only the changed fields', () => {
    const stored = provider()
    expect(toProviderTest(toProviderForm(stored), stored)).toEqual({
      id: stored.id,
    })
    expect(
      toProviderTest(
        {
          ...toProviderForm(stored),
          client_id: 'other',
          client_secret: 'new',
          issuer_url: 'https://other.okta.com',
        },
        stored
      )
    ).toEqual({
      id: stored.id,
      client_id: 'other',
      client_secret: 'new',
      issuer_url: 'https://other.okta.com',
    })
  })
})

describe('ordering', () => {
  it('puts a new provider last', () => {
    expect(nextSortOrder([])).toBe(0)
    expect(nextSortOrder([provider({ sort_order: 4 }), google])).toBe(5)
  })

  it('sorts by position without mutating the input', () => {
    const input = [github, provider(), google]
    expect(sortProviders(input).map(p => p.slug)).toEqual([
      'okta',
      'google',
      'github',
    ])
    expect(input[0]).toBe(github)
  })

  it('moves one place and ignores a move off either end', () => {
    expect(moveProvider(['a', 'b', 'c'], 0, 1)).toEqual(['b', 'a', 'c'])
    expect(moveProvider(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b'])
    expect(moveProvider(['a', 'b', 'c'], 0, -1)).toEqual(['a', 'b', 'c'])
    expect(moveProvider(['a', 'b', 'c'], 2, 1)).toEqual(['a', 'b', 'c'])
    expect(moveProvider(['a', 'b', 'c'], 9, -1)).toEqual(['a', 'b', 'c'])
  })
})

describe('redirectUriFor', () => {
  it('uses the URI the server reported on a stored provider', () => {
    expect(redirectUriFor([provider()])).toEqual({
      value: REDIRECT_URI,
      derived: false,
    })
  })

  it('derives it from the current address when nothing is stored', () => {
    expect(redirectUriFor([])).toEqual({
      value: `${globalThis.location.origin}/api/v1/auth/callback`,
      derived: true,
    })
  })
})

describe('lockout risk', () => {
  it('reads the reason from the problem metadata', () => {
    expect(lockoutRiskReason(lockoutRisk('own_provider'))).toBe('own_provider')
    expect(lockoutRiskReason(lockoutRisk('no_enabled_provider'))).toBe(
      'no_enabled_provider'
    )
  })

  it('still reports a lockout risk that names no reason', () => {
    expect(lockoutRiskReason(apiError(409, 'lockout_risk'))).toBe('unknown')
  })

  it('is null for a version conflict, another status and a plain error', () => {
    expect(lockoutRiskReason(versionConflict())).toBeNull()
    expect(lockoutRiskReason(apiError(400, 'lockout_risk'))).toBeNull()
    expect(lockoutRiskReason(new Error('boom'))).toBeNull()
  })

  it('explains each reason differently', () => {
    const none = lockoutRiskCopy('no_enabled_provider')
    const own = lockoutRiskCopy('own_provider')
    const other = lockoutRiskCopy('something_new')
    expect(none.description).toMatch(/no sign-in provider is enabled/)
    expect(own.description).toMatch(/cannot sign in through it again/)
    expect(other.title).toBe('This change could lock you out')
    expect(new Set([none.title, own.title, other.title]).size).toBe(3)
  })
})

describe('access allowlist', () => {
  it('normalizes the way the server stores a list', () => {
    expect(
      normalizeAllowlistEntries([' Example.com ', 'example.com', '', 'b.io'])
    ).toEqual(['example.com', 'b.io'])
  })

  it.each(['example.com', 'sub.example.co.uk', 'a-b.io'])(
    'accepts the domain %s',
    domain => {
      expect(isAllowlistDomain(domain)).toBe(true)
    }
  )

  it.each([
    'localhost',
    '@example.com',
    'example..com',
    '-a.com',
    'a-.com',
    'exa mple.com',
    `${'a'.repeat(64)}.com`,
    `${'a.'.repeat(127)}com`,
  ])('rejects the domain %j', domain => {
    expect(isAllowlistDomain(domain)).toBe(false)
  })

  it('accepts a bare address at an email domain only', () => {
    expect(isAllowlistEmail('ada@example.com')).toBe(true)
    expect(isAllowlistEmail('ada@localhost')).toBe(false)
    expect(isAllowlistEmail('ada')).toBe(false)
    expect(isAllowlistEmail('ada..b@example.com')).toBe(false)
  })

  it('quotes the first invalid entry of each list', () => {
    expect(
      validateAllowlistForm({
        domains: ['example.com', '@bad'],
        emails: ['ok@example.com', 'nope'],
      })
    ).toEqual({
      domains: expect.stringContaining('"@bad"') as string,
      emails: expect.stringContaining('"nope"') as string,
    })
    expect(
      validateAllowlistForm({ domains: ['example.com'], emails: [] })
    ).toEqual({})
  })

  it('compares lists ignoring order, case and duplicates', () => {
    expect(
      sameAllowlistEntries(['B.io', 'a.io'], ['a.io', 'b.io', 'a.io'])
    ).toBe(true)
    expect(sameAllowlistEntries(['a.io'], ['a.io', 'b.io'])).toBe(false)
  })

  it('spells out the singular', () => {
    expect(allowlistImpactSentence(1)).toBe(
      '1 signed-in user will be signed out'
    )
    expect(allowlistImpactSentence(12)).toBe(
      '12 signed-in users will be signed out'
    )
  })
})
