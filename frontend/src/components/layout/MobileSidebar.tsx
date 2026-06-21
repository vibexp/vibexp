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
import { SheetClose } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

interface MobileGroupProps {
  item: NavItem
  pathname: string
}

function MobileGroup({ item, pathname }: MobileGroupProps) {
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
          <SheetClose asChild key={child.href}>
            {/* Wrap NavLink in a span so Radix's Slot merge doesn't clobber
                NavLink's function `className` prop (it merges classNames and
                only accepts strings, which silently stringifies the function).
                Clicks bubble to the span, so the sheet still closes. */}
            <span className="contents">
              <NavLink
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
            </span>
          </SheetClose>
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}

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
      <ScrollArea className="flex-1">
        <nav className="flex flex-col gap-0.5 px-3.5 pb-2 pt-5">
          {NAV_GROUPS.map(group => (
            <div key={group.label} className="mt-[18px] first:mt-0">
              <div className="text-muted-foreground px-2.5 pb-[7px] text-xs font-bold tracking-wider uppercase">
                {group.label}
              </div>
              {group.items.map(item => {
                const hasChildren = !!item.children?.length
                if (!hasChildren) {
                  return (
                    <SheetClose asChild key={item.href}>
                      {/* Wrap NavLink in a span so Radix's Slot merge doesn't
                          clobber NavLink's function `className` prop (it merges
                          classNames and only accepts strings, which silently
                          stringifies the function). Clicks bubble to the span,
                          so the sheet still closes. */}
                      <span className="contents">
                        <NavLink
                          to={item.href}
                          end={item.href === '/'}
                          className={({ isActive }) =>
                            cn(
                              'flex items-center gap-[9px] rounded-md px-2.5 py-[7px] text-sm font-normal transition-colors',
                              isActive
                                ? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
                                : 'text-sidebar-foreground hover:bg-sidebar-accent/50'
                            )
                          }
                        >
                          <item.icon
                            className="size-[15px] shrink-0 opacity-85"
                            aria-hidden
                          />
                          <span>{item.label}</span>
                        </NavLink>
                      </span>
                    </SheetClose>
                  )
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
    </div>
  )
}
