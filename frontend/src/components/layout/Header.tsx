import { Menu } from 'lucide-react'

import { HeaderBreadcrumb } from '@/components/layout/HeaderBreadcrumb'
import { MobileSidebar } from '@/components/layout/MobileSidebar'
import { NotificationBell } from '@/components/layout/NotificationBell'
import { ProjectSwitcher } from '@/components/layout/ProjectSwitcher'
import { SearchModal } from '@/components/layout/SearchModal'
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

export function Header() {
  return (
    <header className="bg-background sticky top-0 z-30 flex h-14 items-center gap-2 border-b px-4 md:px-6">
      <Sheet>
        <SheetTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden"
            aria-label="Open navigation"
          >
            <Menu className="size-5" />
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-64 p-0">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <MobileSidebar />
        </SheetContent>
      </Sheet>

      <HeaderBreadcrumb />

      <div className="ml-auto flex items-center gap-1.5">
        <TeamSwitcher />
        <ProjectSwitcher />
        <SearchModal />
        <ThemeToggle />
        <NotificationBell />
        <UserMenu />
      </div>
    </header>
  )
}
