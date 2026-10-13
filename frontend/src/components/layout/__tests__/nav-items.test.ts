import { NAV_GROUPS, NAV_ITEMS } from '../nav-items'

function group(label: string) {
  const found = NAV_GROUPS.find(g => g.label === label)
  if (!found) throw new Error(`no nav group labelled ${label}`)
  return found
}

describe('NAV_GROUPS', () => {
  // #538 moved team management out of Settings and onto its own top-level
  // route. The nav entry is load-bearing: HeaderBreadcrumb resolves labels by
  // longest-prefix match over NAV_ITEMS, so removing it silently breaks
  // breadcrumbs on every /teams route as well as the sidebar link.
  it('puts Teams in the System group pointing at /teams', () => {
    const teams = group('System').items.find(i => i.label === 'Teams')
    expect(teams).toBeDefined()
    expect(teams?.href).toBe('/teams')
  })

  // The four setup pages (MCP, CLI, REST API, API client) are tabs of one
  // Connect page: visited once, they no longer take four rows of the sidebar.
  it('has one Connect entry in place of the four integration pages', () => {
    const labels = NAV_ITEMS.map(i => i.label)
    expect(labels).toContain('Connect')
    for (const retired of ['MCP Server', 'CLI', 'REST APIs', 'API Client']) {
      expect(labels).not.toContain(retired)
    }
    expect(NAV_ITEMS.find(i => i.label === 'Connect')?.href).toBe('/connect')
  })

  it('has no Integrations group left', () => {
    expect(NAV_GROUPS.map(g => g.label)).toEqual([
      'General',
      'Workspace',
      'System',
    ])
  })

  it('lists Connect, Teams and Settings under System', () => {
    expect(group('System').items.map(i => i.label)).toEqual([
      'Connect',
      'Teams',
      'Settings',
    ])
  })

  it('gives Teams an icon distinct from Agents', () => {
    // The collapsed icon rail (md-lg) renders icons with no labels, so two
    // entries sharing a glyph are indistinguishable there.
    const teams = NAV_ITEMS.find(i => i.label === 'Teams')
    const agents = NAV_ITEMS.find(i => i.label === 'Agents')
    expect(teams?.icon).toBeDefined()
    expect(agents?.icon).toBeDefined()
    expect(teams?.icon).not.toBe(agents?.icon)
  })

  it('exposes no nav href under the retired /settings/teams path', () => {
    const hrefs = NAV_ITEMS.flatMap(i => [
      i.href,
      ...(i.children ?? []).map(c => c.href),
    ])
    expect(hrefs.filter(h => h.startsWith('/settings/teams'))).toHaveLength(0)
  })

  it('flattens NAV_ITEMS in group order without dropping entries', () => {
    // Compare the actual sequence, not just the count - a length check passes
    // on any permutation, and consumers that render NAV_ITEMS directly depend
    // on the order matching NAV_GROUPS.
    expect(NAV_ITEMS.map(i => i.href)).toEqual(
      NAV_GROUPS.flatMap(g => g.items.map(i => i.href))
    )
  })
})
