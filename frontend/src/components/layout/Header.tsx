import { Info, Menu, PanelLeft, PanelRight } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useLocation } from 'react-router'

import { HeaderBreadcrumb } from '@/components/layout/HeaderBreadcrumb'
import { MobileSidebar } from '@/components/layout/MobileSidebar'
import { NotificationBell } from '@/components/layout/NotificationBell'
import { ProjectSwitcher } from '@/components/layout/ProjectSwitcher'
import { SearchModal } from '@/components/layout/SearchModal'
import { useShell } from '@/components/layout/ShellContext'
import { TeamSwitcher } from '@/components/layout/TeamSwitcher'
import { ThemeToggle } from '@/components/layout/ThemeToggle'
import { UserMenu } from '@/components/layout/UserMenu'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

/**
 * Top bar (#886).
 *
 * - `< md`: a hamburger opens the navigation drawer, which also hosts the
 *   team/project switchers, search and the theme toggle. The bar keeps only
 *   the breadcrumb, notifications and the user menu.
 * - `md–lg`: the navigation is a fixed icon rail, so there is no nav button.
 * - `lg+`: a nav button folds the sidebar between expanded and rail.
 * - A details toggle appears only while a reading page has registered a
 *   details panel: it folds the details column at `lg+` and opens the details
 *   sheet below that.
 */
export function Header() {
  const {
    isTablet,
    isDesktop,
    navExpanded,
    toggleNav,
    detailsRegistered,
    detailsOpen,
    toggleDetails,
    setDetailsSheetOpen,
  } = useShell()
  const { pathname } = useLocation()
  const [drawerOpen, setDrawerOpen] = useState(false)

  // Any navigation (a nav link, a search submit, a switcher change that
  // redirects) closes the drawer, so it never lingers over the new page.
  useEffect(() => {
    setDrawerOpen(false)
  }, [pathname])

  const showDetailsToggle = detailsRegistered
  const detailsPressed = isDesktop ? detailsOpen : undefined
  const DetailsIcon = isDesktop ? PanelRight : Info

  return (
    <header className="bg-background sticky top-0 z-30 flex h-14 items-center gap-2 border-b px-4 md:px-6">
      {!isTablet && (
        <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Open navigation">
              <Menu className="size-5" />
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-72 p-0">
            <SheetTitle className="sr-only">Navigation</SheetTitle>
            <MobileSidebar />
          </SheetContent>
        </Sheet>
      )}

      {isDesktop && (
        <Button
          variant="ghost"
          size="icon"
          aria-label={navExpanded ? 'Collapse navigation' : 'Expand navigation'}
          aria-pressed={!navExpanded}
          data-testid="nav-toggle"
          onClick={toggleNav}
        >
          <PanelLeft className="size-5" />
        </Button>
      )}

      <HeaderBreadcrumb />

      <div className="ml-auto flex min-w-0 items-center gap-1.5">
        {isTablet && (
          <>
            <TeamSwitcher />
            <ProjectSwitcher />
            <SearchModal />
            <ThemeToggle />
          </>
        )}
        <NotificationBell />
        <UserMenu />
        {showDetailsToggle && (
          <Button
            variant={detailsPressed ? 'secondary' : 'ghost'}
            size="icon"
            aria-label={
              isDesktop
                ? detailsOpen
                  ? 'Collapse details'
                  : 'Expand details'
                : 'Open details'
            }
            aria-pressed={detailsPressed}
            data-testid="details-toggle"
            onClick={() => {
              if (isDesktop) toggleDetails()
              else setDetailsSheetOpen(true)
            }}
          >
            <DetailsIcon className="size-5" />
          </Button>
        )}
      </div>
    </header>
  )
}
