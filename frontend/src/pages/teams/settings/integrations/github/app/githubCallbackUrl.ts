/**
 * The GitHub App Callback URL for one team (#587).
 *
 * A sibling module rather than an export from `GitHubAppSetupGuide.tsx`, for
 * the same two reasons `team-settings-cards.ts` gives: a `.tsx` exporting a
 * component and a value trips `react-refresh/only-export-components`, and
 * keeping the value separate makes it testable without rendering the guide.
 */

/**
 * Absolute URL GitHub must return to after an install.
 *
 * `window.location.origin` rather than a configured base URL: this is the
 * origin the admin's browser is actually on, which is the one that has to match
 * what they paste into GitHub. A server-side value could legitimately differ
 * behind a reverse proxy and would be the wrong thing to show.
 *
 * The path is defined by the `settings/integrations/github` route in
 * `pages/teams/TeamRoutes.tsx`. It is spelled out here rather than imported
 * because no shared route-path constant exists yet — `githubCallbackUrl.test.ts`
 * pins it against the settings-hub card's href so the two cannot drift apart
 * silently, which is exactly how this URL went stale in the first place (#541).
 */
export function githubCallbackUrlFor(teamId: string): string {
  return `${window.location.origin}/teams/${teamId}/settings/integrations/github`
}
