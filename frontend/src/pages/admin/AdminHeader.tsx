import { ArrowLeft, Menu } from 'lucide-react'
import { Link } from 'react-router'

import { ThemeToggle } from '@/components/layout/ThemeToggle'
import { UserMenu } from '@/components/layout/UserMenu'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { AdminMobileSidebar } from '@/pages/admin/AdminSidebar'

/**
 * Slim header for the instance-admin portal.
 *
 * Carries only what makes sense at instance scope: the mobile nav drawer, a way
 * back to the product app, the theme toggle and the user menu. Deliberately no
 * `TeamSwitcher`, `ProjectSwitcher`, `SearchModal` or `NotificationBell` — all
 * four are team-scoped, and the switchers require the providers this shell does
 * not mount.
 *
 * `ThemeToggle` and `UserMenu` are reused from `components/layout` because both
 * were verified context-free before this shell dropped the team and project
 * providers; either of them reaching for team context would crash every admin
 * page.
 */
export function AdminHeader() {
  return (
    <header className="bg-background sticky top-0 z-30 flex h-14 items-center gap-2 border-b px-4 md:px-6">
      <Sheet>
        <SheetTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden"
            aria-label="Open admin navigation"
          >
            <Menu className="size-5" />
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-64 p-0">
          <SheetTitle className="sr-only">Admin navigation</SheetTitle>
          <AdminMobileSidebar />
        </SheetContent>
      </Sheet>

      <span className="text-sm font-semibold tracking-tight">Admin Portal</span>

      <div className="ml-auto flex items-center gap-1.5">
        <Button variant="ghost" size="sm" asChild>
          {/* aria-label rather than an sr-only span: the visible text is
              dropped on narrow screens, and the label must survive that. */}
          <Link to="/" aria-label="Back to app">
            <ArrowLeft className="size-4" aria-hidden />
            <span className="hidden sm:inline">Back to app</span>
          </Link>
        </Button>
        <ThemeToggle />
        <UserMenu />
      </div>
    </header>
  )
}
