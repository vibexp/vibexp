import type { AdminSiteInfo } from '@/services/adminSettingsService'

import {
  EMPTY_SITE_INFO_VALUES,
  isHttpUrl,
  SITE_INFO_AUDIT_FIELDS,
  SITE_INFO_FORM_FIELDS,
  toSiteInfoEnvelope,
  toSiteInfoForm,
  toSiteInfoUpdate,
  URL_FIELDS,
  validateSiteInfoForm,
} from '../siteInfoForm'

const limits: AdminSiteInfo['limits'] = {
  company_name_max_length: 120,
  company_email_max_length: 254,
  url_max_length: 2048,
}

const filled: AdminSiteInfo['values'] = {
  company_name: 'Acme Inc.',
  company_email: 'hello@acme.test',
  website: 'https://acme.test',
  support_url: 'https://acme.test/help',
  terms_url: 'https://acme.test/terms',
  privacy_url: 'https://acme.test/privacy',
  logo_url: 'https://cdn.acme.test/logo.svg',
  logo_dark_url: 'https://cdn.acme.test/logo-dark.svg',
}

describe('field lists', () => {
  it('covers all eight wire fields, in form order, in the form and the history', () => {
    expect(SITE_INFO_FORM_FIELDS).toEqual(Object.keys(filled))
    expect(SITE_INFO_AUDIT_FIELDS.map(([key]) => key)).toEqual(
      Object.keys(filled)
    )
    expect(SITE_INFO_AUDIT_FIELDS[7]).toEqual([
      'logo_dark_url',
      'Dark-mode logo URL',
    ])
  })
})

describe('toSiteInfoForm / toSiteInfoUpdate', () => {
  it('round-trips stored values unchanged', () => {
    expect(toSiteInfoUpdate(toSiteInfoForm(filled))).toEqual(filled)
  })

  it('trims every value and sends a blank field as an empty string, never null', () => {
    const update = toSiteInfoUpdate({
      ...EMPTY_SITE_INFO_VALUES,
      company_name: '  Acme Inc.  ',
      website: ' https://acme.test ',
      support_url: '   ',
    })

    expect(update.company_name).toBe('Acme Inc.')
    expect(update.website).toBe('https://acme.test')
    expect(update.support_url).toBe('')
    expect(Object.values(update).every(v => typeof v === 'string')).toBe(true)
  })
})

describe('isHttpUrl', () => {
  it.each(['https://acme.test', 'http://localhost:3000/logo.png'])(
    'accepts %s',
    url => {
      expect(isHttpUrl(url)).toBe(true)
    }
  )

  it.each([
    'acme.test',
    '/logo.png',
    'ftp://acme.test/logo.png',
    'javascript:alert(1)',
    'data:image/png;base64,AAAA',
    'mailto:hello@acme.test',
    'https://',
    '',
  ])('rejects %s', url => {
    expect(isHttpUrl(url)).toBe(false)
  })
})

describe('validateSiteInfoForm', () => {
  it('accepts a fully blank form: every field is optional', () => {
    expect(validateSiteInfoForm(EMPTY_SITE_INFO_VALUES, limits)).toEqual({})
  })

  it('accepts a fully filled form, ignoring surrounding whitespace', () => {
    expect(
      validateSiteInfoForm(
        { ...filled, website: '  https://acme.test  ' },
        limits
      )
    ).toEqual({})
  })

  it.each(['not-an-email', 'a@b.test, c@d.test', 'Acme <hello@acme.test>'])(
    'rejects the email %s',
    email => {
      expect(
        validateSiteInfoForm({ ...filled, company_email: email }, limits)
          .company_email
      ).toMatch(/one email address/i)
    }
  )

  it.each(URL_FIELDS)('rejects a non-http(s) value in %s', field => {
    const errors = validateSiteInfoForm(
      { ...EMPTY_SITE_INFO_VALUES, [field]: 'acme.test/page' },
      limits
    )

    expect(errors).toEqual({
      [field]: 'Enter a full URL starting with http:// or https://.',
    })
  })

  it('checks lengths against the limits the server sent', () => {
    const tight = {
      company_name_max_length: 4,
      company_email_max_length: 10,
      url_max_length: 20,
    }

    expect(
      validateSiteInfoForm(
        {
          ...EMPTY_SITE_INFO_VALUES,
          company_name: 'Acme Inc.',
          company_email: 'hello@acme.test',
          website: 'https://acme.test/a/long/path',
        },
        tight
      )
    ).toEqual({
      company_name: 'Enter at most 4 characters.',
      company_email: 'Enter at most 10 characters.',
      website: 'Enter at most 20 characters.',
    })
  })

  it('counts characters, not UTF-16 units', () => {
    // Four emoji are four characters and eight UTF-16 units.
    expect(
      validateSiteInfoForm(
        { ...EMPTY_SITE_INFO_VALUES, company_name: '🚀🚀🚀🚀' },
        { ...limits, company_name_max_length: 4 }
      )
    ).toEqual({})
  })
})

describe('toSiteInfoEnvelope', () => {
  const stored: AdminSiteInfo = {
    values: filled,
    limits,
    updated_at: '2026-10-10T10:00:00Z',
    updated_by_user_id: '7d3c0b8e-8f7a-4a51-9a3e-1b2c3d4e5f60',
    updated_by_name: 'Ada Admin',
    version: 3,
  }

  it('marks a stored row as the instance’s own, with empty values to fall back to', () => {
    expect(toSiteInfoEnvelope(stored)).toEqual({
      values: filled,
      limits,
      built_in_defaults: EMPTY_SITE_INFO_VALUES,
      source: 'instance',
      teams_with_override: 0,
      updated_at: '2026-10-10T10:00:00Z',
      updated_by_name: 'Ada Admin',
      version: 3,
    })
  })

  it('marks nothing stored (null version) as the default state', () => {
    const envelope = toSiteInfoEnvelope({
      ...stored,
      values: EMPTY_SITE_INFO_VALUES,
      updated_at: null,
      updated_by_user_id: null,
      updated_by_name: null,
      version: null,
    })

    expect(envelope.source).toBe('default')
    expect(envelope.version).toBeNull()
  })
})
