import { EMPTY_FORM } from '@/features/email-provider/emailProviderForm'
import type { AdminInstanceEmailSettings } from '@/services/adminService'

import {
  formFieldForServerField,
  formMatchesStored,
  type InstanceEmailFormValues,
  instanceEmailSchema,
  instanceSecretError,
  sameStoredDestination,
  toInstanceFormValues,
  toInstanceRequest,
} from '../instanceEmailForm'

const unconfigured: AdminInstanceEmailSettings = {
  configured: false,
  provider_type: null,
  has_credential: false,
  is_healthy: null,
}

const storedSMTP = (
  overrides: Partial<AdminInstanceEmailSettings> = {}
): AdminInstanceEmailSettings => ({
  configured: true,
  provider_type: 'smtp',
  settings: {
    smtp: { host: 'smtp.acme.test', port: '587', username: 'mailer' },
  },
  has_credential: true,
  from_address: 'noreply@acme.test',
  from_name: 'Acme',
  reply_to: null,
  contact_recipient_address: 'hello@acme.test',
  privacy_policy_url: 'https://acme.test/privacy',
  is_healthy: true,
  ...overrides,
})

const storedMailgun: AdminInstanceEmailSettings = {
  configured: true,
  provider_type: 'mailgun',
  settings: {
    mailgun: {
      domain: 'mg.acme.test',
      base_url: 'https://api.eu.mailgun.net/v3',
    },
  },
  has_credential: true,
  from_address: 'noreply@acme.test',
  is_healthy: true,
}

const values = (
  overrides: Partial<InstanceEmailFormValues> = {}
): InstanceEmailFormValues => ({
  ...toInstanceFormValues(storedSMTP()),
  ...overrides,
})

describe('toInstanceFormValues', () => {
  it('seeds the shared fields and both instance-only fields, never the secret', () => {
    expect(toInstanceFormValues(storedSMTP())).toEqual({
      ...EMPTY_FORM,
      provider_type: 'smtp',
      from_address: 'noreply@acme.test',
      from_name: 'Acme',
      reply_to: '',
      secret: '',
      smtp_host: 'smtp.acme.test',
      smtp_port: '587',
      smtp_username: 'mailer',
      contact_recipient_address: 'hello@acme.test',
      privacy_policy_url: 'https://acme.test/privacy',
    })
  })

  it('maps null instance-only fields to blank inputs', () => {
    const form = toInstanceFormValues(
      storedSMTP({ contact_recipient_address: null, privacy_policy_url: null })
    )
    expect(form.contact_recipient_address).toBe('')
    expect(form.privacy_policy_url).toBe('')
  })

  it('gives an unconfigured instance the empty form', () => {
    expect(toInstanceFormValues(unconfigured)).toEqual({
      ...EMPTY_FORM,
      contact_recipient_address: '',
      privacy_policy_url: '',
    })
  })
})

describe('toInstanceRequest', () => {
  it('nests the settings and carries both instance-only fields', () => {
    expect(toInstanceRequest(values({ secret: ' pw ' }))).toEqual({
      provider_type: 'smtp',
      from_address: 'noreply@acme.test',
      from_name: 'Acme',
      reply_to: null,
      secret: 'pw',
      settings: {
        smtp: { host: 'smtp.acme.test', port: '587', username: 'mailer' },
      },
      contact_recipient_address: 'hello@acme.test',
      privacy_policy_url: 'https://acme.test/privacy',
    })
  })

  it('clears blank optional fields with an explicit null', () => {
    const request = toInstanceRequest(
      values({
        from_name: '  ',
        reply_to: '',
        contact_recipient_address: '',
        privacy_policy_url: '   ',
      })
    )
    expect(request.from_name).toBeNull()
    expect(request.reply_to).toBeNull()
    expect(request.contact_recipient_address).toBeNull()
    expect(request.privacy_policy_url).toBeNull()
  })

  it('omits a blank secret rather than sending an empty string', () => {
    expect(toInstanceRequest(values({ secret: '  ' }))).not.toHaveProperty(
      'secret'
    )
  })
})

describe('instanceEmailSchema', () => {
  it('accepts a complete configuration', () => {
    expect(instanceEmailSchema.safeParse(values()).success).toBe(true)
  })

  it('keeps the shared rules (an SMTP host is required)', () => {
    const result = instanceEmailSchema.safeParse(values({ smtp_host: '' }))
    expect(result.success).toBe(false)
    expect(result.error?.issues.map(i => i.path.join('.'))).toContain(
      'smtp_host'
    )
  })

  it('rejects a malformed contact recipient', () => {
    const result = instanceEmailSchema.safeParse(
      values({ contact_recipient_address: 'not-an-email' })
    )
    expect(result.error?.issues[0]?.path).toEqual(['contact_recipient_address'])
  })

  it.each(['acme.test/privacy', 'ftp://acme.test/privacy', 'javascript:x'])(
    'rejects the non-http(s) or relative privacy URL %s',
    url => {
      const result = instanceEmailSchema.safeParse(
        values({ privacy_policy_url: url })
      )
      expect(result.error?.issues[0]?.path).toEqual(['privacy_policy_url'])
    }
  )

  it('accepts both instance-only fields blank', () => {
    expect(
      instanceEmailSchema.safeParse(
        values({ contact_recipient_address: '', privacy_policy_url: '' })
      ).success
    ).toBe(true)
  })
})

describe('sameStoredDestination', () => {
  it('is true for the stored SMTP host, port and username', () => {
    expect(sameStoredDestination(storedSMTP(), values())).toBe(true)
  })

  it.each([
    ['host', { smtp_host: 'collector.attacker.test' }],
    ['port', { smtp_port: '2525' }],
    ['username', { smtp_username: 'other' }],
  ])('is false when the SMTP %s changes', (_, change) => {
    expect(sameStoredDestination(storedSMTP(), values(change))).toBe(false)
  })

  it('is false when the provider type changes', () => {
    expect(
      sameStoredDestination(storedSMTP(), values({ provider_type: 'postmark' }))
    ).toBe(false)
  })

  it('compares the Mailgun domain and base URL', () => {
    const form = toInstanceFormValues(storedMailgun)
    expect(sameStoredDestination(storedMailgun, form)).toBe(true)
    expect(
      sameStoredDestination(storedMailgun, {
        ...form,
        mailgun_base_url: 'https://collector.attacker.test/v3',
      })
    ).toBe(false)
  })

  it('treats a vendor-endpoint provider as the same destination by type', () => {
    const stored = storedSMTP({ provider_type: 'sendgrid', settings: {} })
    expect(
      sameStoredDestination(stored, values({ provider_type: 'sendgrid' }))
    ).toBe(true)
  })

  it('is false when nothing is stored', () => {
    expect(sameStoredDestination(unconfigured, values())).toBe(false)
  })
})

describe('instanceSecretError', () => {
  it('never blocks an action that carries a secret', () => {
    const form = values({ secret: 'pw', provider_type: 'postmark' })
    expect(instanceSecretError('save', unconfigured, form)).toBeNull()
    expect(instanceSecretError('test', unconfigured, form)).toBeNull()
  })

  describe('save', () => {
    it('requires a credential for a first configuration', () => {
      expect(instanceSecretError('save', unconfigured, values())).toBe(
        'A credential is required'
      )
    })

    it('keeps the stored credential for the same provider type', () => {
      expect(instanceSecretError('save', storedSMTP(), values())).toBeNull()
    })

    it('keeps it even when the SMTP host changes (the save is audited)', () => {
      expect(
        instanceSecretError(
          'save',
          storedSMTP(),
          values({ smtp_host: 'smtp2.acme.test' })
        )
      ).toBeNull()
    })

    it('requires a new credential when the provider type changes', () => {
      expect(
        instanceSecretError(
          'save',
          storedSMTP(),
          values({ provider_type: 'sendgrid' })
        )
      ).toMatch(/new provider’s credential/)
    })
  })

  describe('test', () => {
    it('borrows the stored credential for the same destination', () => {
      expect(instanceSecretError('test', storedSMTP(), values())).toBeNull()
    })

    it('borrows it for an SMTP relay stored without a credential', () => {
      expect(
        instanceSecretError(
          'test',
          storedSMTP({ has_credential: false }),
          values()
        )
      ).toBeNull()
    })

    it('requires a credential for a different destination', () => {
      expect(
        instanceSecretError(
          'test',
          storedSMTP(),
          values({ smtp_host: 'collector.attacker.test' })
        )
      ).toMatch(/different destination/)
    })

    it('requires a credential when nothing is stored', () => {
      expect(instanceSecretError('test', unconfigured, values())).toBe(
        'Enter the credential to send a test.'
      )
    })
  })
})

describe('formFieldForServerField', () => {
  it.each([
    ['settings.smtp.host', 'smtp_host'],
    ['settings.smtp.port', 'smtp_port'],
    ['settings.mailgun.domain', 'mailgun_domain'],
    ['contact_recipient_address', 'contact_recipient_address'],
    ['privacy_policy_url', 'privacy_policy_url'],
    ['secret', 'secret'],
  ])('maps %s onto the %s input', (server, formField) => {
    expect(formFieldForServerField(server)).toBe(formField)
  })

  it('maps a whole-block error to no input', () => {
    expect(formFieldForServerField('settings.smtp')).toBeUndefined()
  })
})

describe('formMatchesStored', () => {
  it('is true for the untouched seeded form', () => {
    expect(formMatchesStored(storedSMTP(), values())).toBe(true)
  })

  it.each([
    ['a changed field', { from_name: 'Other' }],
    ['a typed secret', { secret: 'pw' }],
    ['a changed instance-only field', { privacy_policy_url: 'https://x.test' }],
  ])('is false for %s', (_, change) => {
    expect(formMatchesStored(storedSMTP(), values(change))).toBe(false)
  })
})
