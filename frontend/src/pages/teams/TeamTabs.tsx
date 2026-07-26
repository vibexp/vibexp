import { NavLink } from 'react-router-dom'

import { cn } from '@/lib/utils'
import { teamTabsFor } from '@/pages/teams/team-tabs'

/**
 * Horizontal tab bar shown across `/teams/:id/**` (#539).
 *
 * `NavLink` rather than the Radix `Tabs` primitive: these tabs navigate, they
 * do not switch in-page panels, so routing must own the active state. NavLink
 * also sets `aria-current="page"` for free, which is what the tests assert.
 *
 * The rule the tabs sit on belongs to `TeamScopeLayout`, which draws it edge to
 * edge of the main region (#666); `-mb-px` is what lets the active tab's
 * indicator overlap that rule rather than float a pixel above it.
 *
 * Only the hovered and the active tab carry an indicator. An idle tab still
 * reserves the 2px (`border-transparent`) so hovering does not shift the label,
 * and the two visible states are a contrast ramp against the rule they sit on:
 * `--muted-foreground` (hover) -> `--foreground` (active). Neither is
 * `--border` — that is the rule's own grey, so it would read as the rule
 * thickening rather than as a state.
 */
export function TeamTabs({ teamId }: Readonly<{ teamId: string }>) {
  return (
    <nav aria-label="Team sections" className="-mb-px">
      <ul className="flex gap-6">
        {teamTabsFor(teamId).map(tab => (
          <li key={tab.href}>
            <NavLink
              to={tab.href}
              end={tab.end}
              className={({ isActive }) =>
                cn(
                  'inline-block border-b-2 pb-3 text-sm transition-colors',
                  'focus-visible:ring-ring rounded-t-sm focus-visible:outline-none focus-visible:ring-2',
                  isActive
                    ? 'border-foreground text-foreground font-semibold'
                    : 'text-muted-foreground hover:text-foreground hover:border-muted-foreground border-transparent font-medium'
                )
              }
            >
              {tab.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}
