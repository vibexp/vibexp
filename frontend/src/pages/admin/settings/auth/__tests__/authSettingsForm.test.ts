import {
  adminValuesOf,
  allowlistImpactSentence,
  availableProviderTypes,
  emptyProviderForm,
  isAllowlistDomain,
  isAllowlistEmail,
  issuerUrlError,
  lockoutRiskCopy,
  lockoutRiskReason,
  lockoutRiskReasons,
  lockoutRisksCopy,
  moveProvider,
  nextSortOrder,
  normalizeAllowlistEntries,
  redirectUriFor,
  roleMappingOf,
  rolesClaimError,
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

describe('role mapping (#1645)', () => {
  const oidc = {
    ...emptyProviderForm('oidc'),
    slug: 'okta',
    display_name: 'Okta',
    client_id: 'abc',
    client_secret: 's3cret',
    issuer_url: 'https://example.okta.com',
  }
  const mapped = provider({
    roles_claim: 'groups',
    admin_values: ['platform-admins', 'VibeXP Admins'],
  })

  it('keeps each value exactly, one per line, dropping blanks and duplicates', () => {
    expect(
      adminValuesOf(
        'platform-admins\n\n  VibeXP Admins \r\nplatform-admins\nplatform-Admins'
      )
    ).toEqual(['platform-admins', 'VibeXP Admins', 'platform-Admins'])
  })

  it('turns the mapping off with a blank claim', () => {
    expect(roleMappingOf({ roles_claim: ' ', admin_values: 'x' })).toEqual({
      roles_claim: null,
      admin_values: [],
    })
    expect(
      roleMappingOf({ roles_claim: ' realm_access.roles ', admin_values: 'a' })
    ).toEqual({ roles_claim: 'realm_access.roles', admin_values: ['a'] })
  })

  it('accepts a dotted or URL-namespaced claim path only', () => {
    expect(rolesClaimError('groups')).toBeNull()
    expect(rolesClaimError('realm_access.roles')).toBeNull()
    expect(rolesClaimError('https://example.com/roles')).toBeNull()
    expect(rolesClaimError('realm_access..roles')).toMatch(/dotted path/)
    expect(rolesClaimError('.groups')).toMatch(/dotted path/)
    expect(rolesClaimError('my groups')).toMatch(/spaces/)
    expect(rolesClaimError('g'.repeat(256))).toMatch(/at most 255/)
  })

  it('needs a claim and values together, and is optional', () => {
    expect(validateProviderForm(oidc, null)).toEqual({})
    expect(
      validateProviderForm({ ...oidc, roles_claim: 'groups' }, null)
    ).toEqual({
      admin_values: 'Enter at least one value that makes a user an admin.',
    })
    expect(
      validateProviderForm({ ...oidc, admin_values: 'platform-admins' }, null)
    ).toEqual({ roles_claim: 'Enter the claim that carries these values.' })
    expect(
      validateProviderForm(
        { ...oidc, roles_claim: 'groups', admin_values: 'platform-admins' },
        null
      )
    ).toEqual({})
  })

  it('caps the number and length of the values', () => {
    const many = Array.from({ length: 51 }, (_, i) => `g${String(i)}`)
    expect(
      validateProviderForm(
        { ...oidc, roles_claim: 'groups', admin_values: many.join('\n') },
        null
      ).admin_values
    ).toBe('Enter at most 50 values.')
    expect(
      validateProviderForm(
        { ...oidc, roles_claim: 'groups', admin_values: 'a'.repeat(256) },
        null
      ).admin_values
    ).toMatch(/at most 255 characters/)
  })

  it('never judges a mapping on Google or GitHub', () => {
    const form = {
      ...emptyProviderForm('google'),
      client_id: 'abc',
      client_secret: 'x',
      admin_values: 'stale',
    }
    expect(validateProviderForm(form, null)).toEqual({})
    expect(toProviderCreate(form, 0, 1)).not.toHaveProperty('admin_values')
  })

  it('round-trips a stored mapping through the form', () => {
    const form = toProviderForm(mapped)
    expect(form.roles_claim).toBe('groups')
    expect(form.admin_values).toBe('platform-admins\nVibeXP Admins')
    expect(roleMappingOf(form)).toEqual({
      roles_claim: 'groups',
      admin_values: ['platform-admins', 'VibeXP Admins'],
    })
  })

  it('creates a provider with its mapping, and without one when blank', () => {
    expect(
      toProviderCreate(
        {
          ...oidc,
          roles_claim: 'groups',
          admin_values: 'platform-admins\nVibeXP Admins',
        },
        0,
        1
      )
    ).toMatchObject({
      roles_claim: 'groups',
      admin_values: ['platform-admins', 'VibeXP Admins'],
    })
    const plain = toProviderCreate(oidc, 0, 1)
    expect(plain).not.toHaveProperty('roles_claim')
    expect(plain).not.toHaveProperty('admin_values')
  })

  it('sends a set or edited mapping on update', () => {
    const set = toProviderUpdate(
      provider(),
      {
        ...toProviderForm(provider()),
        roles_claim: 'groups',
        admin_values: 'platform-admins',
      },
      4,
      false
    )
    expect(set).toMatchObject({
      roles_claim: 'groups',
      admin_values: ['platform-admins'],
    })
    const edited = toProviderUpdate(
      mapped,
      { ...toProviderForm(mapped), admin_values: 'VibeXP Admins' },
      4,
      false
    )
    expect(edited).toMatchObject({
      roles_claim: 'groups',
      admin_values: ['VibeXP Admins'],
    })
  })

  it('clears the mapping with a null claim and no values', () => {
    const body = toProviderUpdate(
      mapped,
      { ...toProviderForm(mapped), roles_claim: '', admin_values: '' },
      4,
      false
    )
    expect(body.roles_claim).toBeNull()
    expect(body.admin_values).toEqual([])
  })

  it('omits an untouched mapping, so the stored one is kept', () => {
    const unchanged = toProviderUpdate(mapped, toProviderForm(mapped), 4, false)
    const toggled = toProviderUpdate(mapped, { enabled: false }, 4, false)
    const moved = toProviderUpdate(mapped, { sort_order: 3 }, 4, false)
    for (const body of [unchanged, toggled, moved]) {
      expect(body).not.toHaveProperty('roles_claim')
      expect(body).not.toHaveProperty('admin_values')
    }
  })

  it('never sends a mapping for a Google provider', () => {
    const body = toProviderUpdate(
      google,
      { ...toProviderForm(google), roles_claim: 'groups', admin_values: 'x' },
      4,
      false
    )
    expect(body).not.toHaveProperty('roles_claim')
    expect(body).not.toHaveProperty('admin_values')
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
  it('lists every reason the server sends, keeping only non-empty strings (#1473)', () => {
    expect(
      lockoutRiskReasons(
        lockoutRisk('own_method', {
          reasons: ['own_method', '', 3, null, 'password_only_accounts'],
        })
      )
    ).toEqual(['own_method', 'password_only_accounts'])
  })

  it.each([
    ['no list', {}],
    ['an empty list', { reasons: [] }],
    ['a list of nothing usable', { reasons: [1, null, ''] }],
    ['not a list', { reasons: 'own_method' }],
  ])(
    'falls back to the single reason when the server sends %s',
    (_case, metadata) => {
      expect(lockoutRiskReasons(lockoutRisk('own_method', metadata))).toEqual([
        'own_method',
      ])
    }
  )

  it('is empty for any other error', () => {
    expect(lockoutRiskReasons(versionConflict())).toEqual([])
    expect(lockoutRiskReasons(new Error('boom'))).toEqual([])
  })

  it('titles one reason as that reason, and several as a group', () => {
    expect(lockoutRisksCopy(['own_method']).title).toBe(
      'You signed in with a password'
    )
    const several = lockoutRisksCopy(['own_method', 'something_new'])
    expect(several.title).toBe(
      'This change could lock people out in several ways'
    )
    expect(several.items).toHaveLength(2)
  })

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
    const admin = lockoutRiskCopy('own_admin_status')
    const other = lockoutRiskCopy('something_new')
    expect(none.description).toMatch(/no sign-in provider is enabled/)
    expect(own.description).toMatch(/cannot sign in through it again/)
    expect(admin.title).toBe('This removes your own admin access')
    expect(admin.description).toMatch(/only through this provider's role/)
    expect(other.title).toBe('This change could lock you out')
    expect(
      new Set([none.title, own.title, admin.title, other.title]).size
    ).toBe(4)
  })

  it('counts the password-only accounts in the title', () => {
    expect(lockoutRiskCopy('password_only_accounts', 1).title).toBe(
      '1 account can only sign in with a password'
    )
    expect(lockoutRiskCopy('password_only_accounts', 3).title).toBe(
      '3 accounts can only sign in with a password'
    )
    expect(lockoutRiskCopy('password_only_accounts').title).toBe(
      'Some accounts can only sign in with a password'
    )
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
