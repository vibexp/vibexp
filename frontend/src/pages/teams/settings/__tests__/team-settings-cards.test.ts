import { teamSettingsCardsFor } from '../team-settings-cards'

describe('teamSettingsCardsFor', () => {
  it('lists the four team-scoped configuration pages relocated in #540', () => {
    expect(teamSettingsCardsFor('team-a').map(c => c.title)).toEqual([
      'Search Settings',
      'Model Providers',
      'Embedding Providers',
      'Artifact Types',
    ])
  })

  it('points each card at its own route segment', () => {
    expect(teamSettingsCardsFor('team-a').map(c => c.href)).toEqual([
      '/teams/team-a/settings/search',
      '/teams/team-a/settings/model-providers',
      '/teams/team-a/settings/embedding-providers',
      '/teams/team-a/settings/customization',
    ])
  })

  it('scopes every card href to the given team id', () => {
    // The mistake this guards: an href missing the id would send the user to
    // whichever team happened to be ambient - the exact problem epic #536
    // exists to remove. Stays meaningful as #541 adds more cards.
    for (const card of teamSettingsCardsFor('team-b')) {
      expect(card.href).toMatch(/^\/teams\/team-b\/settings\//)
    }
  })

  it('keeps the "Artifact Types" title for the customization route', () => {
    // `customization` is the route segment only; the user-facing name is
    // Artifact Types, exactly as on the personal hub.
    const card = teamSettingsCardsFor('team-a').find(c =>
      c.href.endsWith('/customization')
    )
    expect(card?.title).toBe('Artifact Types')
  })

  it('gives every card a distinct icon', () => {
    // Cards are visually indistinguishable if two share a glyph.
    const icons = teamSettingsCardsFor('team-a').map(c => c.icon)
    expect(new Set(icons).size).toBe(icons.length)
  })
})
