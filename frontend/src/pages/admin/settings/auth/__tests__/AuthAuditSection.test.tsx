import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'
import { authSettingsService } from '@/services/authSettingsService'

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: { listAudit: vi.fn() },
}))

import { AuthAuditSection } from '../AuthAuditSection'
import {
  AUTH_AUDIT_SECTIONS,
  authAuditActorLabel,
  authAuditChanges,
  clientSecretChange,
  madeFromCli,
} from '../authSettingsAudit'

const service = vi.mocked(authSettingsService)

const entry = (
  overrides: Partial<AdminInstanceSettingsAuditEntry>
): AdminInstanceSettingsAuditEntry => ({
  id: 'e1',
  setting: 'auth_providers',
  action: 'upsert',
  actor_user_id: 'u1',
  actor_name: 'Ada Admin',
  before: null,
  after: null,
  created_at: '2026-10-10T10:00:00Z',
  ...overrides,
})

const sectionOf = (setting: string) => {
  const section = AUTH_AUDIT_SECTIONS.find(s => s.setting === setting)
  if (!section) throw new Error(`no section ${setting}`)
  return section
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('authSettingsAudit', () => {
  it('covers the four authentication settings', () => {
    expect(AUTH_AUDIT_SECTIONS.map(s => s.setting)).toEqual([
      'auth_providers',
      'auth_allowlist',
      'instance_admins',
      'auth_setup',
    ])
  })

  it('diffs only the fields an admin sets on a provider', () => {
    const changes = authAuditChanges(
      {
        before: {
          slug: 'okta',
          enabled: true,
          client_id: 'a',
          updated_at: '2026-10-01T00:00:00Z',
        },
        after: {
          slug: 'okta',
          enabled: false,
          client_id: 'a',
          client_secret: 'unchanged',
          updated_at: '2026-10-02T00:00:00Z',
        },
      },
      sectionOf('auth_providers')
    )
    expect(changes).toEqual([
      { key: 'enabled', label: 'Enabled', before: 'Yes', after: 'No' },
    ])
  })

  it('shows an allowlist as comma-separated lists', () => {
    expect(
      authAuditChanges(
        {
          before: { domains: ['a.io'], emails: [] },
          after: { domains: ['a.io', 'b.io'], emails: [] },
        },
        sectionOf('auth_allowlist')
      )
    ).toEqual([
      {
        key: 'domains',
        label: 'Allowed domains',
        before: 'a.io',
        after: 'a.io, b.io',
      },
    ])
  })

  it("labels each setting's actions in its own terms", () => {
    expect(sectionOf('auth_providers').actionLabel('delete')).toBe('Deleted')
    expect(sectionOf('auth_providers').actionLabel('import')).toBe('Imported')
    expect(sectionOf('auth_allowlist').actionLabel('delete')).toBe(
      'Reset to open access'
    )
    expect(sectionOf('instance_admins').actionLabel('upsert')).toBe('Granted')
    expect(sectionOf('instance_admins').actionLabel('delete')).toBe('Revoked')
    expect(sectionOf('auth_setup').actionLabel('upsert')).toBe('Recorded')
  })

  it('reads the client secret marker and nothing else as one', () => {
    expect(clientSecretChange({ after: { client_secret: 'changed' } })).toBe(
      'changed'
    )
    expect(clientSecretChange({ after: { client_secret: 'unchanged' } })).toBe(
      'unchanged'
    )
    expect(
      clientSecretChange({ after: { client_secret: 's3cret' } })
    ).toBeNull()
    expect(clientSecretChange({ after: null })).toBeNull()
  })

  it('finds the CLI marker on a save and on a removal', () => {
    expect(madeFromCli({ before: null, after: { source: 'cli' } })).toBe(true)
    expect(madeFromCli({ before: { source: 'cli' }, after: null })).toBe(true)
    expect(madeFromCli({ before: { slug: 'okta' }, after: null })).toBe(false)
  })
})

describe('authAuditActorLabel', () => {
  const noActor = { actor_user_id: null, actor_name: null }

  it('names the admin who made the change', () => {
    expect(authAuditActorLabel(entry({ after: { slug: 'okta' } }))).toBe(
      'Ada Admin'
    )
  })

  it.each(['auth_providers', 'auth_allowlist'] as const)(
    'cannot tell a setup session from a deleted admin on %s, and says so',
    setting => {
      expect(
        authAuditActorLabel(
          entry({ ...noActor, setting, after: { slug: 'okta' } })
        )
      ).toBe('Setup session or deleted user')
    }
  )

  it('reads an actorless admin grant as a deleted user: no setup session can make one', () => {
    expect(
      authAuditActorLabel(
        entry({
          ...noActor,
          setting: 'instance_admins',
          after: { user_id: 'u9' },
        })
      )
    ).toBe('Deleted user')
  })

  it('reads an actorless CLI save and CLI removal as the server CLI', () => {
    expect(
      authAuditActorLabel(entry({ ...noActor, after: { source: 'cli' } }))
    ).toBe('Server CLI')
    expect(
      authAuditActorLabel(
        entry({ ...noActor, action: 'delete', before: { source: 'cli' } })
      )
    ).toBe('Server CLI')
    expect(
      authAuditActorLabel(
        entry({
          ...noActor,
          setting: 'auth_setup',
          after: { event: 'rearmed', source: 'cli' },
        })
      )
    ).toBe('Server CLI')
  })

  it.each(['token_minted', 'rearmed'])(
    'attributes a %s setup event to the server',
    event => {
      expect(
        authAuditActorLabel(
          entry({ ...noActor, setting: 'auth_setup', after: { event } })
        )
      ).toBe('Server')
    }
  )

  it('reads an actorless completed setup as a deleted user: a root admin completed it', () => {
    expect(
      authAuditActorLabel(
        entry({
          ...noActor,
          setting: 'auth_setup',
          after: { event: 'consumed' },
        })
      )
    ).toBe('Deleted user')
  })

  it('keeps the boot-time import label', () => {
    expect(authAuditActorLabel(entry({ ...noActor, action: 'import' }))).toBe(
      'Imported from config.yaml'
    )
  })
})

describe('AuthAuditSection', () => {
  it('labels an actorless change and a CLI change by what the entry supports', async () => {
    service.listAudit.mockResolvedValue({
      entries: [
        entry({
          id: 'setup',
          actor_user_id: null,
          actor_name: null,
          after: { slug: 'okta', enabled: true },
        }),
        entry({
          id: 'cli',
          actor_user_id: null,
          actor_name: null,
          before: { slug: 'okta', enabled: true },
          after: { slug: 'okta', enabled: false, source: 'cli' },
        }),
      ],
      next_cursor: null,
    })
    render(<AuthAuditSection refreshKey={0} />)

    const rows = await screen.findAllByTestId('auth-audit-entry-auth_providers')
    expect(rows[0]).toHaveTextContent('Setup session or deleted user')
    expect(rows[1]).toHaveTextContent('Server CLI')
    expect(rows[1]).toHaveTextContent('via CLI')
  })

  it('loads the provider history first, with its badges', async () => {
    service.listAudit.mockResolvedValue({
      entries: [
        entry({
          before: { slug: 'okta', enabled: true },
          after: {
            slug: 'okta',
            enabled: false,
            client_secret: 'changed',
            source: 'cli',
          },
        }),
      ],
      next_cursor: null,
    })
    render(<AuthAuditSection refreshKey={0} />)

    const row = await screen.findByTestId('auth-audit-entry-auth_providers')
    expect(row).toHaveTextContent('Saved')
    expect(row).toHaveTextContent('Ada Admin')
    expect(row).toHaveTextContent('Client secret changed')
    expect(row).toHaveTextContent('via CLI')
    expect(within(row).getByText('Enabled')).toBeVisible()
    expect(service.listAudit).toHaveBeenCalledWith('auth_providers', {
      limit: 20,
    })
    expect(service.listAudit).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['Allowlist', 'auth_allowlist'],
    ['Admins', 'instance_admins'],
    ['Setup', 'auth_setup'],
  ] as const)('the %s tab loads the %s history', async (label, setting) => {
    service.listAudit.mockImplementation(requested =>
      Promise.resolve({
        entries:
          requested === setting
            ? [
                entry({
                  setting,
                  action: 'delete',
                  before: { domains: ['a.io'], user_id: 'u9', event: 'x' },
                }),
              ]
            : [],
        next_cursor: null,
      })
    )
    const user = userEvent.setup()
    render(<AuthAuditSection refreshKey={0} />)
    await screen.findByText('No changes have been recorded yet.')

    await user.click(screen.getByRole('tab', { name: label }))
    expect(
      await screen.findByTestId(`auth-audit-entry-${setting}`)
    ).toBeVisible()
    expect(service.listAudit).toHaveBeenLastCalledWith(setting, { limit: 20 })
  })

  it('reloads the open history when the page records a change', async () => {
    service.listAudit.mockResolvedValue({ entries: [], next_cursor: null })
    const { rerender } = render(<AuthAuditSection refreshKey={0} />)
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(1)
    })
    rerender(<AuthAuditSection refreshKey={1} />)
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(2)
    })
  })
})
