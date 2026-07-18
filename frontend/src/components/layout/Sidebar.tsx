import { ChevronRight } from 'lucide-react'
import { NavLink, useLocation } from 'react-router-dom'

import { NAV_GROUPS, type NavItem } from '@/components/layout/nav-items'
import { SidebarBrand } from '@/components/layout/SidebarBrand'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/**
 * Responsive sidebar.
 *
 * - `< md` (< 768px): hidden. Header renders a mobile `Sheet` hamburger.
 * - `md`–`lg` (768–1024px): 60px icon rail. Labels hidden, tooltips on
 *   hover. Collapsible groups flatten to a single icon-link that
 *   navigates to the parent's href.
 * - `lg+` (≥ 1024px): 256px expanded. Labels, collapsible children,
 *   chevrons as before.
 */
const rowClass = (active: boolean) =>
  cn(
    'flex items-center gap-[9px] rounded-md py-[7px] text-sm font-normal transition-colors',
    'justify-center px-0 lg:justify-start lg:px-2.5',
    active
      ? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
      : 'text-sidebar-foreground hover:bg-sidebar-accent/50'
  )

function RailLinkWithTooltip({ item }: Readonly<{ item: NavItem }>) {
  const Icon = item.icon
  return (
    <Tooltip>
      {/* Wrap NavLink in a span so Radix's Slot merge doesn't clobber
          NavLink's function `className` prop (it merges classNames and
          only accepts strings, which silently stringifies the function). */}
      <TooltipTrigger asChild>
        <span className="contents">
          <NavLink
            to={item.href}
            end={item.href === '/'}
            className={({ isActive }) => rowClass(isActive)}
          >
            <Icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
            <span className="hidden lg:inline">{item.label}</span>
          </NavLink>
        </span>
      </TooltipTrigger>
      {/* Only surface the tooltip at the rail breakpoint where labels
          are hidden. Above `lg` the tooltip would repeat the label text. */}
      <TooltipContent side="right" className="lg:hidden">
        {item.label}
      </TooltipContent>
    </Tooltip>
  )
}

function RailGroupLink({
  item,
  pathname,
}: Readonly<{
  item: NavItem
  pathname: string
}>) {
  const Icon = item.icon
  const isActive =
    pathname === item.href ||
    pathname.startsWith(item.href + '/') ||
    (item.children ?? []).some(c => pathname.startsWith(c.href))
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="contents">
          <NavLink
            to={item.href}
            className={cn(
              'flex items-center justify-center gap-2 rounded-md px-0 py-2 text-sm transition-colors lg:hidden',
              isActive
                ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                : 'hover:bg-sidebar-accent/50'
            )}
          >
            <Icon className="size-[15px] shrink-0 opacity-85" aria-hidden />
          </NavLink>
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{item.label}</TooltipContent>
    </Tooltip>
  )
}

function ExpandedGroup({
  item,
  pathname,
}: Readonly<{
  item: NavItem
  pathname: string
}>) {
  const Icon = item.icon
  const children = item.children ?? []
  const isGroupOpen =
    pathname === item.href ||
    pathname.startsWith(item.href + '/') ||
    children.some(c => pathname.startsWith(c.href))

  return (
    <Collapsible defaultOpen={isGroupOpen} className="hidden lg:block">
      <CollapsibleTrigger
        className={cn(
          rowClass(isGroupOpen),
          'w-full cursor-pointer justify-between'
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
          <NavLink
            key={child.href}
            to={child.href}
            className={({ isActive }) =>
              cn(
                'rounded-md px-3 py-1.5 text-sm transition-colors',
                isActive
                  ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium'
                  : 'text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-foreground'
              )
            }
          >
            {child.label}
          </NavLink>
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}

export function Sidebar() {
  const { pathname } = useLocation()

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
          <nav className="flex flex-col gap-0.5 px-2 pb-2 pt-5 lg:px-3.5">
            {NAV_GROUPS.map(group => (
              <div key={group.label} className="mt-[18px] first:mt-0">
                <div className="text-muted-foreground hidden px-2.5 pb-[7px] text-xs font-bold tracking-wider uppercase lg:block">
                  {group.label}
                </div>
                {group.items.map(item => {
                  const hasChildren = !!item.children?.length
                  if (!hasChildren) {
                    return <RailLinkWithTooltip key={item.href} item={item} />
                  }
                  return (
                    <div key={item.href}>
                      <ExpandedGroup item={item} pathname={pathname} />
                      <RailGroupLink item={item} pathname={pathname} />
                    </div>
                  )
                })}
              </div>
            ))}
          </nav>
        </ScrollArea>
      </aside>
    </TooltipProvider>
  )
}
