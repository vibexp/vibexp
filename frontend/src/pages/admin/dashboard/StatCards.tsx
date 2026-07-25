import type { LucideIcon } from 'lucide-react'
import {
  BookOpen,
  Bot,
  FileText,
  FolderKanban,
  HardDrive,
  KeyRound,
  Package,
  Radio,
  Users,
  UsersRound,
} from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { AdminExtendedCounts } from '@/services/adminService'

interface CountCard {
  key: keyof AdminExtendedCounts
  label: string
  icon: LucideIcon
}

/** Ordered so the entities an admin asks about first come first. */
const CARDS: CountCard[] = [
  { key: 'users', label: 'Users', icon: Users },
  { key: 'teams', label: 'Teams', icon: UsersRound },
  { key: 'projects', label: 'Projects', icon: FolderKanban },
  { key: 'prompts', label: 'Prompts', icon: FileText },
  { key: 'artifacts', label: 'Artifacts', icon: Package },
  { key: 'memories', label: 'Memories', icon: HardDrive },
  { key: 'blueprints', label: 'Blueprints', icon: BookOpen },
  { key: 'agents', label: 'Agents', icon: Bot },
  { key: 'feeds', label: 'Feeds', icon: Radio },
  { key: 'api_keys', label: 'API keys', icon: KeyRound },
]

export function StatCards({
  counts,
  loading,
}: Readonly<{ counts: AdminExtendedCounts | null; loading: boolean }>) {
  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-5">
        {CARDS.map(card => (
          <Skeleton
            key={card.key}
            data-testid="stat-skeleton"
            className="h-24 w-full"
          />
        ))}
      </div>
    )
  }
  if (!counts) return null

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 xl:grid-cols-5">
      {CARDS.map(({ key, label, icon: Icon }) => (
        <Card key={key}>
          <CardHeader className="pb-2">
            <CardTitle className="text-muted-foreground flex items-center gap-2 text-xs font-medium">
              <Icon className="size-4" aria-hidden />
              {label}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold tabular-nums">
              {counts[key].toLocaleString()}
            </p>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
