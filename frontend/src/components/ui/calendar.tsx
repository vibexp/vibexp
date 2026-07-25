import { ChevronLeft, ChevronRight } from 'lucide-react'
import * as React from 'react'
import { DayPicker } from 'react-day-picker'

import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * Calendar primitive — a thin wrapper over `react-day-picker`'s `DayPicker`,
 * styled entirely through its `classNames` slots with the same semantic tokens
 * the other `ui/*` primitives use (`accent`, `muted`, `primary`, `ring`).
 *
 * The library's own stylesheet is deliberately **not** imported: mixing it with
 * these class names would produce two competing sets of spacing and colours, and
 * its palette is not token-aware, so dark mode would break. Everything the
 * calendar needs to lay out is expressed here.
 *
 * Props pass straight through to `DayPicker`, so selection mode, `numberOfMonths`,
 * `defaultMonth`, `disabled` and the rest behave exactly as documented upstream.
 */
export type CalendarProps = React.ComponentProps<typeof DayPicker>

export function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  ...props
}: Readonly<CalendarProps>) {
  const navButton = cn(
    buttonVariants({ variant: 'outline' }),
    'size-7 bg-transparent p-0 opacity-50 hover:opacity-100'
  )

  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      className={cn('p-3', className)}
      classNames={{
        months: 'flex flex-col gap-4 sm:flex-row',
        month: 'flex flex-col gap-4',
        month_caption: 'flex h-7 items-center justify-center',
        caption_label: 'text-sm font-medium',
        nav: 'absolute inset-x-3 top-3 flex items-center justify-between',
        button_previous: navButton,
        button_next: navButton,
        chevron: 'size-4',
        month_grid: 'w-full border-collapse space-y-1',
        weekdays: 'flex',
        weekday: 'text-muted-foreground w-9 text-[0.8rem] font-normal',
        week: 'mt-2 flex w-full',
        day: cn(
          'relative p-0 text-center text-sm focus-within:relative focus-within:z-20',
          // The range background must meet edge-to-edge between cells, so the
          // rounding lives on the range ends rather than on every day.
          '[&:has([aria-selected])]:bg-accent',
          '[&:has(>.day-range-start)]:rounded-l-md [&:has(>.day-range-end)]:rounded-r-md'
        ),
        day_button: cn(
          buttonVariants({ variant: 'ghost' }),
          'size-9 p-0 font-normal aria-selected:opacity-100'
        ),
        range_start:
          'day-range-start bg-primary text-primary-foreground rounded-l-md',
        range_end:
          'day-range-end bg-primary text-primary-foreground rounded-r-md',
        range_middle: 'bg-accent text-accent-foreground rounded-none',
        selected: 'bg-primary text-primary-foreground',
        today: 'bg-accent text-accent-foreground rounded-md',
        outside: 'text-muted-foreground opacity-50',
        disabled: 'text-muted-foreground opacity-50',
        hidden: 'invisible',
        ...classNames,
      }}
      components={{
        // Own chevron so the arrows match every other icon in the app rather
        // than the library's inline SVG. Only `className` and `orientation` are
        // forwarded: the library also passes `size` and `disabled`, and handing
        // `disabled` to an <svg> makes React warn about a non-boolean attribute.
        Chevron: ({ className: chevronClass, orientation }) => {
          const Icon = orientation === 'left' ? ChevronLeft : ChevronRight
          return <Icon className={cn('size-4', chevronClass)} />
        },
      }}
      {...props}
    />
  )
}
