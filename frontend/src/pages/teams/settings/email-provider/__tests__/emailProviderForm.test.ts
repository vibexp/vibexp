import type { TeamEmailProviderResponse } from '@/services/emailProviderService'

import {
  type EmailProviderFormValues,
  emailProviderSchema,
  EMPTY_FORM,
  PROVIDER_TYPES,
  providerTypeMeta,
  secretError,
  toFormValues,
  toRequest,
} from '../emailProviderForm'

const valid = (
  overrides: Partial<EmailProviderFormValues> = {}
): EmailProviderFormValues => ({
  ...EMPTY_FORM,
  from_address: 'hello@acme.test',
  smtp_host: 'smtp.acme.test',
  smtp_port: '587',
  ...overrides,
})

describe('emailProviderSchema', () => {
  it('accepts a complete SMTP configuration', () => {
    expect(emailProviderSchema.safeParse(valid()).success).toBe(true)
  })

  it('requires a valid from address', () => {
    expect(
      emailProviderSchema.safeParse(valid({ from_address: 'not-an-address' }))
        .success
    ).toBe(false)
  })

  it('accepts a blank Reply-To but rejects a malformed one', () => {
    expect(emailProviderSchema.safeParse(valid({ reply_to: '' })).success).toBe(
      true
    )
    expect(
      emailProviderSchema.safeParse(valid({ reply_to: 'nope' })).success
    ).toBe(false)
  })

  it.each([
    ['host', { smtp_host: '' }],
    ['port', { smtp_port: '' }],
  ])('requires the SMTP %s', (_label, overrides) => {
    expect(emailProviderSchema.safeParse(valid(overrides)).success).toBe(false)
  })

  it.each(['0', '70000', 'abc', '58.5'])(
    'rejects the out-of-range SMTP port %s',
    port => {
      expect(
        emailProviderSchema.safeParse(valid({ smtp_port: port })).success
      ).toBe(false)
    }
  )

  it('ignores SMTP fields once another provider type is selected', () => {
    // Switching type must not leave the form permanently invalid because of
    // values belonging to the type the admin moved away from.
    const values = valid({
      provider_type: 'sendgrid',
      smtp_host: '',
      smtp_port: '',
    })
    expect(emailProviderSchema.safeParse(values).success).toBe(true)
  })

  it('requires a Mailgun domain and rejects a pasted URL', () => {
    const base = valid({
      provider_type: 'mailgun',
      smtp_host: '',
      smtp_port: '',
    })
    expect(emailProviderSchema.safeParse(base).success).toBe(false)
    expect(
      emailProviderSchema.safeParse({
        ...base,
        mailgun_domain: 'https://app.mailgun.com/mg.acme.test',
      }).success
    ).toBe(false)
    expect(
      emailProviderSchema.safeParse({ ...base, mailgun_domain: 'mg.acme.test' })
        .success
    ).toBe(true)
  })

  it('needs nothing beyond the sender identity for Postmark and SendGrid', () => {
    for (const provider_type of ['postmark', 'sendgrid'] as const) {
      const values = valid({ provider_type, smtp_host: '', smtp_port: '' })
      expect(emailProviderSchema.safeParse(values).success).toBe(true)
    }
  })
})

describe('toFormValues', () => {
  const configured: TeamEmailProviderResponse = {
    configured: true,
    source: 'team',
    effective_from_address: 'hello@acme.test',
    provider_type: 'mailgun',
    has_credential: true,
    from_address: 'hello@acme.test',
    from_name: 'Acme Team',
    reply_to: 'support@acme.test',
    settings: {
      mailgun: {
        domain: 'mg.acme.test',
        base_url: 'https://api.eu.mailgun.net/v3',
      },
    },
  }

  it('seeds every field from a configured provider', () => {
    expect(toFormValues(configured)).toEqual({
      ...EMPTY_FORM,
      provider_type: 'mailgun',
      from_address: 'hello@acme.test',
      from_name: 'Acme Team',
      reply_to: 'support@acme.test',
      mailgun_domain: 'mg.acme.test',
      mailgun_base_url: 'https://api.eu.mailgun.net/v3',
    })
  })

  it('always blanks the secret, since no response can carry one', () => {
    expect(toFormValues(configured).secret).toBe('')
  })

  it('returns the empty form when the team inherits the instance provider', () => {
    // Notably it does NOT pre-fill effective_from_address: that address belongs
    // to the instance, and a team's own provider is usually not authorized to
    // send for it, so offering it would invite a hard-bouncing configuration.
    expect(
      toFormValues({
        configured: false,
        source: 'instance',
        effective_from_address: 'noreply@instance.test',
        provider_type: null,
        has_credential: false,
      })
    ).toEqual(EMPTY_FORM)
  })
})

describe('toRequest', () => {
  it('nests only the settings block matching the selected type', () => {
    // The backend rejects a block belonging to another type rather than
    // ignoring it, so leftovers from switching types must not be sent.
    const request = toRequest(
      valid({
        provider_type: 'mailgun',
        mailgun_domain: 'mg.acme.test',
        smtp_host: 'left.over.test',
        smtp_port: '25',
      })
    )
    expect(request.settings).toEqual({ mailgun: { domain: 'mg.acme.test' } })
    expect(request.settings).not.toHaveProperty('smtp')
  })

  it('sends no settings block at all for SendGrid', () => {
    const request = toRequest(valid({ provider_type: 'sendgrid' }))
    expect(request.settings).toBeUndefined()
  })

  it('omits a blank secret so the stored credential survives', () => {
    // An empty string is REJECTED by the backend rather than treated as
    // "clear", so an untouched field must produce no key at all.
    expect(toRequest(valid({ secret: '' }))).not.toHaveProperty('secret')
    expect(toRequest(valid({ secret: '   ' }))).not.toHaveProperty('secret')
    expect(toRequest(valid({ secret: ' hunter2 ' })).secret).toBe('hunter2')
  })

  it('omits blank optional identity fields', () => {
    const request = toRequest(valid({ from_name: '', reply_to: '' }))
    expect(request).not.toHaveProperty('from_name')
    expect(request).not.toHaveProperty('reply_to')
  })

  it('omits optional per-type fields when blank', () => {
    expect(toRequest(valid({ smtp_username: '' })).settings?.smtp).toEqual({
      host: 'smtp.acme.test',
      port: '587',
    })
    expect(
      toRequest(
        valid({ provider_type: 'postmark', postmark_message_stream: '' })
      ).settings
    ).toEqual({})
  })

  it('trims what it does send', () => {
    const request = toRequest(
      valid({
        from_address: '  hello@acme.test  ',
        smtp_host: ' smtp.acme.test ',
      })
    )
    expect(request.from_address).toBe('hello@acme.test')
    expect(request.settings?.smtp?.host).toBe('smtp.acme.test')
  })
})

describe('secretError', () => {
  it('lets an unchanged secret through when saving a configured provider', () => {
    expect(secretError('save', true, '')).toBeNull()
  })

  it('requires a secret when configuring a provider for the first time', () => {
    expect(secretError('save', false, '')).not.toBeNull()
  })

  it('always requires a secret to test, even on a configured provider', () => {
    // The test endpoint sends with the request body, not the stored config, so
    // there is no credential to fall back on.
    expect(secretError('test', true, '')).not.toBeNull()
    expect(secretError('test', true, '   ')).not.toBeNull()
    expect(secretError('test', true, 'hunter2')).toBeNull()
  })
})

describe('providerTypeMeta', () => {
  it('names a distinct credential for every supported type', () => {
    const labels = PROVIDER_TYPES.map(p => providerTypeMeta(p.id).secretLabel)
    expect(labels).toHaveLength(4)
    expect(new Set(labels).size).toBe(4)
  })
})
