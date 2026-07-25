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
    expect(NAV_ITEMS).toHaveLength(
      NAV_GROUPS.reduce((n, g) => n + g.items.length, 0)
    )
  })
})
