import type { SettingItem } from '@/components/settings/SettingsGrid'

/**
 * The cards shown on the team settings hub, for one team (#539).
 *
 * A sibling data module rather than a const inside `TeamSettings.tsx`, for the
 * same two reasons as `team-tabs.ts`: a `.tsx` exporting a component and a
 * value trips `react-refresh/only-export-components`, and keeping the data
 * separate makes it testable — and mockable — without rendering the page.
 *
 * **Empty today, by design.** Every team-scoped settings page still lives under
 * `/settings/**` until #540 (search, model providers, embedding providers,
 * artifact types) and #541 (GitHub integration) relocate them. Those issues each
 * add their entries here and nothing else on the hub has to change — that is the
 * point of routing the hub through the shared `SettingSection`.
 */
export function teamSettingsCardsFor(teamId: string): SettingItem[] {
  // Keep `teamId` in the signature: every future card's href is built from it,
  // and #540/#541 should not have to change this function's shape to add one.
  void teamId
  return []
}
