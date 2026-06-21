import { ChevronRight, type LucideIcon } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

export interface QuickAction {
  title: string
  description: string
  icon: LucideIcon
  to: string
  buttonText: string
}

/** A Quick-actions card: dark icon tile, title, description, and a CTA button
 * that navigates to the action's route. */
export function QuickActionCard({ action }: { action: QuickAction }) {
  const Icon = action.icon
  const navigate = useNavigate()
  return (
    <Card>
      <CardHeader>
        <div className="bg-primary text-primary-foreground flex size-10 items-center justify-center rounded-md">
          <Icon className="size-5" />
        </div>
        <CardTitle className="mt-3 text-base">{action.title}</CardTitle>
        <CardDescription>{action.description}</CardDescription>
      </CardHeader>
      <CardFooter>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            void navigate(action.to)
          }}
        >
          {action.buttonText}
          <ChevronRight className="ml-1 size-4" />
        </Button>
      </CardFooter>
    </Card>
  )
}
