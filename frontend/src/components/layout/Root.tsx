import * as React from 'react'

import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

export function Root({ children }: Readonly<{ children: React.ReactNode }>) {
  const { resolvedTheme } = useTheme()

  return (
    <div
      className={cn('v2-root min-h-screen', resolvedTheme === 'dark' && 'dark')}
      data-theme={resolvedTheme}
    >
      {children}
    </div>
  )
}
