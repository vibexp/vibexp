import { ChevronRight } from 'lucide-react'
import { NavLink, useLocation } from 'react-router'

import { NAV_GROUPS, type NavItem } from '@/components/layout/nav-items'
import { ProjectSwitcher } from '@/components/layout/ProjectSwitcher'
import { SearchModal } from '@/components/layout/SearchModal'
import { SidebarBrand } from '@/components/layout/SidebarBrand'
import { TeamSwitcher } from '@/components/layout/TeamSwitcher'
import { ThemeToggle } from '@/components/layout/ThemeToggle'
import { useNavLinkActive } from '@/components/layout/useNavLinkActive'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SheetClose } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

/**
 * A slotted `asChild` child must be a real element with a real box, never a
 * `display: contents` wrapper: Radix's `Slot` merges onto whatever it is given,
 * and for a popper trigger that element is also the anchor floating-ui measures
 * (a box-less wrapper measures 0x0 at the viewport origin — #891). `SheetClose`
 * only needs click bubbling, which a real `<a>` provides, so these rows are not
 * broken today; they carry the same anti-pattern and are normalised with the
 * rails so it cannot be copied forward.
 *
 * Slot also string-joins `className`, which stringifies `NavLink`'s function
 * form into garbage classes — hence `useNavLinkActive`, which mirrors NavLink's
 * own `isActive` so the string form highlights identically. These have to be
 * components rather than inline JSX because the links are rendered from a
 * `.map()`, where a hook may not be called.
 */
function DrawerLeafLink({ item }: Readonly<{ item: NavItem }>) {
  const end = item.href === '/'
  const active = useNavLinkActive(item.href, end)
  return (
    <SheetClose asChild>
      <NavLink
        to={item.href}
        end={end}
        className={cn(
          'flex items-center gap-[9px] rounded-md px-2.5 py-[7px] text-sm font-normal transition-colors',
          active
            ? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
            : 'text-sidebar-foreground hover:bg-sidebar-accent/50'
        )}
      >
        <item.icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
        <span>{item.label}</span>
      </NavLink>
    </SheetClose>
  )
}

/** Child row of a collapsible group. See `DrawerLeafLink` for the shape. */
function DrawerChildLink({
  child,
}: Readonly<{ child: NonNullable<NavItem['children']>[number] }>) {
  const active = useNavLinkActive(child.href)
  return (
    <SheetClose asChild>
      <NavLink
        to={child.href}
        className={cn(
          'rounded-md px-3 py-1.5 text-sm transition-colors',
          active
            ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium'
            : 'text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground'
        )}
      >
        {child.label}
      </NavLink>
    </SheetClose>
  )
}

interface MobileGroupProps {
  item: NavItem
  pathname: string
}

function MobileGroup({ item, pathname }: Readonly<MobileGroupProps>) {
  const Icon = item.icon
  const children = item.children ?? []
  const isGroupOpen =
    pathname === item.href ||
    pathname.startsWith(item.href + '/') ||
    children.some(c => pathname.startsWith(c.href))

  return (
    <Collapsible defaultOpen={isGroupOpen}>
      <CollapsibleTrigger
        className={cn(
          'flex w-full cursor-pointer items-center gap-[9px] rounded-md px-2.5 py-[7px] text-sm font-normal transition-colors',
          isGroupOpen
            ? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
            : 'text-sidebar-foreground hover:bg-sidebar-accent/50',
          'justify-between'
        )}
      >
        <span className="flex items-center gap-[9px]">
          <Icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
          <span>{item.label}</span>
        </span>
        <ChevronRight className="size-4 opacity-50 transition-transform data-[state=open]:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent className="ml-6 mt-0.5 flex flex-col gap-0.5 border-l pl-2">
        {children.map(child => (
          <DrawerChildLink key={child.href} child={child} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}

/**
 * The mobile navigation drawer (#886). Besides the nav it hosts what the
 * mobile header gives up for space: the team and project switchers at the
 * top, search and the theme toggle in the footer.
 */
export function MobileSidebar() {
  const { pathname } = useLocation()

  return (
    <div className="bg-sidebar text-sidebar-foreground flex h-full flex-col">
      <SheetClose asChild>
        {/* Extra top padding so the logo isn't cramped against the sheet's
            top edge / close button (the shared brand's default py-3 is too
            tight for the mobile sheet). */}
        <SidebarBrand showText className="pt-5" />
      </SheetClose>
      {/* Stacked, not side by side: each switcher keeps its own trigger width
          (up to 220px), and two of them do not fit a 288px drawer. */}
      <div
        className="flex flex-col items-start gap-2 px-3.5 pb-3"
        data-testid="drawer-switchers"
      >
        <TeamSwitcher />
        <ProjectSwitcher />
      </div>
      <ScrollArea className="flex-1">
        <nav className="flex flex-col gap-0.5 px-3.5 pb-2 pt-3">
          {NAV_GROUPS.map(group => (
            <div key={group.label} className="mt-[18px] first:mt-0">
              <div className="text-muted-foreground px-2.5 pb-[7px] text-xs font-bold tracking-wider uppercase">
                {group.label}
              </div>
              {group.items.map(item => {
                const hasChildren = !!item.children?.length
                if (!hasChildren) {
                  return <DrawerLeafLink key={item.href} item={item} />
                }
                return (
                  <MobileGroup
                    key={item.href}
                    item={item}
                    pathname={pathname}
                  />
                )
              })}
            </div>
          ))}
        </nav>
      </ScrollArea>
      <div
        className="flex items-center gap-1 border-t px-3.5 py-2"
        data-testid="drawer-footer"
      >
        <SearchModal />
        <ThemeToggle />
      </div>
    </div>
  )
}
