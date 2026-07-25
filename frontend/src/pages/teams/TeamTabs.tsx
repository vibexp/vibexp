import { NavLink } from 'react-router-dom'

import { cn } from '@/lib/utils'
import { teamTabsFor } from '@/pages/teams/team-tabs'

/**
 * Horizontal tab bar shown across `/teams/:id/**` (#539).
 *
 * `NavLink` rather than the Radix `Tabs` primitive: these tabs navigate, they
 * do not switch in-page panels, so routing must own the active state. NavLink
 * also sets `aria-current="page"` for free, which is what the tests assert.
 */
export function TeamTabs({ teamId }: Readonly<{ teamId: string }>) {
  return (
    <nav aria-label="Team sections" className="border-border -mb-px border-b">
      <ul className="flex gap-1">
        {teamTabsFor(teamId).map(tab => (
          <li key={tab.href}>
            <NavLink
              to={tab.href}
              end={tab.end}
              className={({ isActive }) =>
                cn(
                  'inline-block border-b-2 px-3 py-2 text-sm transition-colors',
                  'focus-visible:ring-ring rounded-t-md focus-visible:outline-none focus-visible:ring-2',
                  isActive
                    ? 'border-primary text-foreground font-semibold'
                    : 'text-muted-foreground hover:text-foreground border-transparent'
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
