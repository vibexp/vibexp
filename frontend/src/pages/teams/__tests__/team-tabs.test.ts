import { teamTabsFor } from '../team-tabs'

describe('teamTabsFor', () => {
  it('builds every href from the given team id', () => {
    expect(teamTabsFor('team-42').map(t => t.href)).toEqual([
      '/teams/team-42',
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

  it('omits a Projects tab until the route exists (#542)', () => {
    // Deliberate: /teams/:id/projects has no route yet, and a tab landing on
    // the in-shell not-found page is a visibly broken affordance. #542 adds
    // the tab with the route. Delete this test when it does.
    expect(teamTabsFor('team-42').map(t => t.label)).not.toContain('Projects')
  })
})
