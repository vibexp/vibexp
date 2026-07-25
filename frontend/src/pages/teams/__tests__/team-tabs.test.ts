import { teamTabsFor } from '../team-tabs'

describe('teamTabsFor', () => {
  it('builds every href from the given team id', () => {
    expect(teamTabsFor('team-42').map(t => t.href)).toEqual([
      '/teams/team-42',
      '/teams/team-42/projects',
      '/teams/team-42/analytics',
      '/teams/team-42/settings',
    ])
  })

  it('marks only the Overview tab as an exact match', () => {
    // Without `end`, Overview stays active on /analytics and /settings too,
    // because its href is a prefix of both.
    const tabs = teamTabsFor('team-42')
    expect(tabs.filter(t => t.end).map(t => t.label)).toEqual(['Overview'])
  })
})
