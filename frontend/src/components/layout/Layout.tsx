import type { ReactNode } from 'react'

import { PendingInvitationsBanner } from '@/components/invitations/PendingInvitationsBanner'
import { Header } from '@/components/layout/Header'
import { ShellProvider, useShell } from '@/components/layout/ShellContext'
import { Sidebar } from '@/components/layout/Sidebar'

/**
 * The app shell (#886).
 *
 * Layout contract:
 * - Sidebar: hidden on mobile (drawer from the header), 60px icon rail at
 *   md–lg (768–1024px); at lg+ (≥ 1024px) the user toggles it between the
 *   264px expanded state and the icon rail from the header. Remembered per
 *   browser.
 * - Content has two modes, chosen by the page (see `ShellContext`):
 *   - `contained` (default): centered, capped at `max-w-screen-xl` (1280px)
 *     with padding scaling from `px-4` on mobile to `px-8` at lg+. Pages
 *     should NOT add their own `mx-auto max-w-*` wrapper.
 *   - `reading`: full-bleed. The page (via `ReadingPage`) owns the content
 *     row and lays out the article column and the details column itself.
 *     The row is NOT capped or centered by the shell: the design pins the
 *     details column flush to the right edge of the content area and centers
 *     the article in whatever is left beside it (#890). Capping the row
 *     instead (#888/#889) floated the column away from the edge and pushed
 *     the article against the row's left edge.
 * - Individual pages render content with their own vertical rhythm.
 */
export function Layout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <ShellProvider>
      <ShellFrame>{children}</ShellFrame>
    </ShellProvider>
  )
}

function ShellFrame({ children }: Readonly<{ children: ReactNode }>) {
  const { contentMode } = useShell()

  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Header />
        {contentMode === 'reading' ? (
          <main
            className="flex min-w-0 flex-1 flex-col"
            data-content-mode="reading"
          >
            {/* The banner renders nothing when there is nothing pending, so the
                padded wrapper collapses via `empty:hidden`. */}
            <div className="px-4 pt-6 empty:hidden md:px-6 lg:px-8">
              <PendingInvitationsBanner />
            </div>
            {/* Full width, so the details column can sit flush against the
                right edge of the content area. No `overflow`, `transform` or
                `filter` here: any of them would become the containing block
                for the rail's `position: sticky`. */}
            <div
              className="flex min-h-0 w-full min-w-0 flex-1"
              data-testid="reading-row"
            >
              {children}
            </div>
          </main>
        ) : (
          <main className="flex-1 overflow-auto" data-content-mode="contained">
            <div className="mx-auto w-full max-w-screen-xl px-4 py-6 md:px-6 lg:px-8">
              <PendingInvitationsBanner />
              {children}
            </div>
          </main>
        )}
      </div>
    </div>
  )
}
