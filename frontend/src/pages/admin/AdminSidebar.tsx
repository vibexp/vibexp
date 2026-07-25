import { NavLink } from 'react-router-dom'

import { SidebarBrand } from '@/components/layout/SidebarBrand'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SheetClose } from '@/components/ui/sheet'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { ADMIN_NAV_ITEMS, type AdminNavItem } from '@/pages/admin/admin-nav'

/**
 * Left navigation for the instance-admin portal.
 *
 * Deliberately a sibling of `components/layout/Sidebar.tsx` rather than a
 * variant of it: the admin area is instance-scoped, has no team/project
 * concepts and no collapsible groups, and will evolve its own information
 * architecture (epic #450). A shared base component would mean every future
 * admin change had to prove it did not disturb the product sidebar.
 *
 * What IS shared is the visual language — `SidebarBrand`, the same `ui/*`
 * primitives, the same `sidebar-*` tokens and the same breakpoints as the main
 * sidebar, so the two never read as different products:
 * - `< md`: hidden; `AdminHeader` renders the same nav in a `Sheet` drawer.
 * - `md`–`lg`: 60px icon rail, labels hidden, tooltips on hover.
 * - `lg+`: 264px expanded with labels.
 */
const rowClass = (active: boolean) =>
  cn(
    'flex items-center gap-[9px] rounded-md py-[7px] text-sm font-normal transition-colors',
    active
      ? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
      : 'text-sidebar-foreground hover:bg-sidebar-accent/50'
  )

/** Rail row: centred icon below `lg`, icon + label at `lg+`, tooltip only where the label is hidden. */
function RailNavRow({ item }: Readonly<{ item: AdminNavItem }>) {
  const Icon = item.icon
  return (
    <Tooltip>
      {/* Wrap NavLink in a span so Radix's Slot merge cannot clobber NavLink's
          function `className` prop — Slot merges classNames and only accepts
          strings, silently stringifying the function. Same guard as the main
          sidebar. */}
      <TooltipTrigger asChild>
        <span className="contents">
          <NavLink
            to={item.href}
            end={item.end}
            className={({ isActive }) =>
              cn(
                rowClass(isActive),
                'justify-center px-0 lg:justify-start lg:px-2.5'
              )
            }
          >
            <Icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
            <span className="hidden lg:inline">{item.label}</span>
          </NavLink>
        </span>
      </TooltipTrigger>
      {/* Above `lg` the label is visible, so the tooltip would only repeat it. */}
      <TooltipContent side="right" className="lg:hidden">
        {item.label}
      </TooltipContent>
    </Tooltip>
  )
}

/**
 * Drawer row: always labelled, and wrapped in `SheetClose` so tapping a link
 * closes the drawer. `SheetClose` needs a `Sheet` ancestor, which is why the
 * drawer gets its own row component instead of a prop on the rail one.
 */
function DrawerNavRow({ item }: Readonly<{ item: AdminNavItem }>) {
  const Icon = item.icon
  return (
    <SheetClose asChild>
      <span className="contents">
        <NavLink
          to={item.href}
          end={item.end}
          className={({ isActive }) => cn(rowClass(isActive), 'px-2.5')}
        >
          <Icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
          <span>{item.label}</span>
        </NavLink>
      </span>
    </SheetClose>
  )
}

/**
 * The admin nav list, rendered from the single `ADMIN_NAV_ITEMS` source so the
 * rail and the mobile drawer can never drift apart.
 */
export function AdminNavList({
  variant = 'rail',
}: Readonly<{ variant?: 'rail' | 'drawer' }>) {
  const Row = variant === 'drawer' ? DrawerNavRow : RailNavRow
  return (
    <nav
      aria-label="Admin sections"
      className={cn(
        'flex flex-col gap-0.5 pb-2 pt-5',
        variant === 'drawer' ? 'px-3.5' : 'px-2 lg:px-3.5'
      )}
    >
      {/* Hidden in the 60px rail, where there is no width for it — same
          treatment as the product sidebar's group labels. */}
      <div
        className={cn(
          'text-muted-foreground px-2.5 pb-[7px] text-xs font-bold tracking-wider uppercase',
          variant === 'drawer' ? 'block' : 'hidden lg:block'
        )}
      >
        Administration
      </div>
      {ADMIN_NAV_ITEMS.map(item => (
        <Row key={item.href} item={item} />
      ))}
    </nav>
  )
}

/** Mobile drawer body — same brand block and nav as the rail, always expanded. */
export function AdminMobileSidebar() {
  return (
    <div className="bg-sidebar text-sidebar-foreground flex h-full flex-col">
      <SheetClose asChild>
        {/* Extra top padding so the logo isn't cramped against the sheet's
            close button, matching the product drawer. */}
        <SidebarBrand showText className="pt-5" />
      </SheetClose>
      <ScrollArea className="flex-1">
        <AdminNavList variant="drawer" />
      </ScrollArea>
    </div>
  )
}

export function AdminSidebar() {
  return (
    <TooltipProvider delayDuration={0}>
      <aside
        className={cn(
          'bg-sidebar text-sidebar-foreground hidden shrink-0 border-r md:flex md:flex-col',
          'w-[60px] lg:w-[264px]'
        )}
      >
        <SidebarBrand />
        <ScrollArea className="flex-1">
          <AdminNavList />
        </ScrollArea>
      </aside>
    </TooltipProvider>
  )
}
