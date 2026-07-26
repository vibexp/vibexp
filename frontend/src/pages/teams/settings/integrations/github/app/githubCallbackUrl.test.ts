/**
 * The GitHub App Callback URL is the one value in the whole integration flow
 * that lives ONLY on github.com (#587). VibeXP cannot read it, cannot write it,
 * and cannot detect that it is wrong — a stale one makes every install fail
 * silently. So the value the guide tells an admin to paste has to be right, and
 * has to stay right the next time this page moves.
 *
 * `settings/integrations/github` is spelled out in two places: the route in
 * `pages/teams/TeamRoutes.tsx` and the hub card's href in
 * `team-settings-cards.ts`. This suite pins the guide's URL against the card so
 * the two cannot drift apart unnoticed — which is precisely what happened in
 * #541, when the page moved and the Callback URL silently went stale.
 */
import { teamSettingsCardsFor } from '@/pages/teams/settings/team-settings-cards'

import { githubCallbackUrlFor } from './GitHubAppSetupGuide'

const TEAM_ID = '11111111-2222-4333-8444-555555555555'

describe('githubCallbackUrlFor', () => {
  it('is absolute, team-scoped and rooted at the browser origin', () => {
    expect(githubCallbackUrlFor(TEAM_ID)).toBe(
      `${window.location.origin}/teams/${TEAM_ID}/settings/integrations/github`
    )
  })

  it('never emits the retired non-team-scoped path', () => {
    const url = githubCallbackUrlFor(TEAM_ID)

    // The path epic #536 retired. Anchored on the origin so the assertion
    // cannot be satisfied by the correct `/teams/<id>/settings/...` form.
    expect(url).not.toBe(
      `${window.location.origin}/settings/integrations/github`
    )
    expect(url).toContain(`/teams/${TEAM_ID}/`)
  })

  it('matches the settings-hub card href for the same team', () => {
    // The drift guard. If the route moves again and only one of the two call
    // sites is updated, this fails instead of shipping an install flow that
    // dead-ends on github.com.
    const card = teamSettingsCardsFor(TEAM_ID).find(
      item => item.title === 'GitHub Integration'
    )
    expect(card).toBeDefined()

    expect(githubCallbackUrlFor(TEAM_ID)).toBe(
      `${window.location.origin}${card?.href ?? ''}`
    )
  })

  it('scopes to the team it is given', () => {
    expect(githubCallbackUrlFor('team-a')).not.toBe(
      githubCallbackUrlFor('team-b')
    )
  })
})
