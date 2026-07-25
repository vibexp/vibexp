import { teamSettingsCardsFor } from '../team-settings-cards'

describe('teamSettingsCardsFor', () => {
  it('has no cards yet — pages relocate here in #540 and #541', () => {
    // Not a placeholder assertion: it pins the hub's empty state to a single
    // data source, so #540/#541 extend this function and its test together
    // rather than touching the page's markup.
    expect(teamSettingsCardsFor('team-a')).toEqual([])
  })

  it('scopes every card it returns to the given team id', () => {
    // Vacuous while the list is empty, and deliberately so: it fails the moment
    // #540 adds a card whose href is not team-scoped, which is the mistake that
    // would let the hub link into whichever team happened to be ambient.
    for (const card of teamSettingsCardsFor('team-a')) {
      expect(card.href).toContain('/teams/team-a/')
    }
  })
})
