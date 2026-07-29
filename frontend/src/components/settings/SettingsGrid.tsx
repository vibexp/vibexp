import { ArrowRight } from 'lucide-react'
import type { ComponentType } from 'react'
import { useNavigate } from 'react-router'

import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

export interface SettingItem {
  title: string
  description: string
  icon: ComponentType<{ className?: string }>
  href: string
}

export function SettingSection({
  title,
  items,
}: Readonly<{
  title: string
  items: SettingItem[]
}>) {
  const navigate = useNavigate()
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {items.map(item => {
          const Icon = item.icon
          return (
            <Card
              key={item.href}
              role="button"
              tabIndex={0}
              className={cn(
                'group relative cursor-pointer transition-colors',
                'hover:border-primary/40',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
              )}
              onClick={() => {
                void navigate(item.href)
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  void navigate(item.href)
                }
              }}
            >
              <ArrowRight className="text-muted-foreground absolute right-4 top-4 size-4 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
              <CardHeader>
                <div className="bg-muted text-foreground mb-2 flex size-10 items-center justify-center rounded-md">
                  <Icon className="size-5" />
                </div>
                <CardTitle className="text-base">{item.title}</CardTitle>
                <CardDescription>{item.description}</CardDescription>
              </CardHeader>
            </Card>
          )
        })}
      </div>
    </section>
  )
}
