import type { ReactNode } from 'react'

import { PendingInvitationsBanner } from '@/components/invitations/PendingInvitationsBanner'
import { Header } from '@/components/layout/Header'
import { Sidebar } from '@/components/layout/Sidebar'

/**
 * v2 app shell.
 *
 * Layout contract:
 * - Sidebar: hidden on mobile (hamburger Sheet), 60px icon rail at
 *   md–lg (768–1024px), 256px expanded at lg+ (≥ 1024px).
 * - Content: centered, capped at `max-w-screen-xl` (1280px) so line
 *   lengths and table widths stay readable on wide monitors. Padding
 *   scales from `px-4` on mobile to `px-8` at lg+. Pages should NOT
 *   add their own `mx-auto max-w-*` wrapper — the layout owns the
 *   container. Long-form prose blocks (markdown bodies) may still
 *   cap themselves further with `max-w-prose` for line-length comfort.
 * - Individual pages render content directly with their own vertical
 *   rhythm (e.g. `space-y-6`).
 */
export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Header />
        <main className="flex-1 overflow-auto">
          <div className="mx-auto w-full max-w-screen-xl px-4 py-6 md:px-6 lg:px-8">
            <PendingInvitationsBanner />
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}
