import { activeTeamTab } from '../team-tabs'

describe('activeTeamTab', () => {
  it.each([
    ['/teams/t1/projects', 'Projects'],
    ['/teams/t1/projects/p9/edit', 'Projects'],
    ['/teams/t1/analytics', 'Analytics'],
    ['/teams/t1/settings/freshness', 'Settings'],
  ])('finds the tab %s is on', (path, label) => {
    expect(activeTeamTab('t1', path)?.label).toBe(label)
  })

  it('is undefined on the team’s own page: Overview adds no crumb', () => {
    expect(activeTeamTab('t1', '/teams/t1')).toBeUndefined()
  })

  it('does not match another team’s paths or a look-alike prefix', () => {
    expect(activeTeamTab('t1', '/teams/t2/projects')).toBeUndefined()
    expect(activeTeamTab('t1', '/teams/t1/projects-archive')).toBeUndefined()
  })
})
