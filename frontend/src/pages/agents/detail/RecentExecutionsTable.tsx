import { Activity } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AgentExecution } from '@/services/agentService'

import { formatDuration, formatRelativeTime } from '../helpers'

interface RecentExecutionsTableProps {
  recentExecutions: AgentExecution[]
  loadingExecutions: boolean
  agentId: string
}

function executionStatusVariant(
  status: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'success':
    case 'completed':
      return 'default'
    case 'error':
    case 'failed':
      return 'destructive'
    case 'running':
    case 'working':
      return 'secondary'
    default:
      return 'outline'
  }
}

function getUserMessage(input: unknown): string {
  if (!input) return 'N/A'
  let text: string
  if (typeof input === 'object' && 'text' in input) {
    text = String(input.text)
  } else if (typeof input === 'string') {
    text = input
  } else {
    text = JSON.stringify(input)
  }
  return text.length > 100 ? `${text.substring(0, 100)}…` : text
}

export function RecentExecutionsTable({
  recentExecutions,
  loadingExecutions,
  agentId,
}: Readonly<RecentExecutionsTableProps>) {
  const navigate = useNavigate()

  const renderExecutions = () => {
    if (recentExecutions.length === 0) {
      return (
        <p className="text-muted-foreground py-6 text-center text-sm">
          No executions yet
        </p>
      )
    }
    return (
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>User message</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Started</TableHead>
              <TableHead className="text-right">Duration</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {recentExecutions.map(execution => (
              <TableRow key={execution.id}>
                <TableCell className="max-w-md">
                  <div className="truncate text-sm">
                    {getUserMessage(execution.input)}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={executionStatusVariant(execution.status)}>
                    {execution.status}
                  </Badge>
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {formatRelativeTime(execution.started_at)}
                </TableCell>
                <TableCell className="text-right text-sm">
                  {formatDuration(execution.duration)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">Recent tasks</CardTitle>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            void navigate(`/agents/${agentId}/tasks`)
          }}
        >
          <Activity className="mr-2 size-4" />
          View all tasks
        </Button>
      </CardHeader>
      <CardContent>
        {loadingExecutions ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : (
          renderExecutions()
        )}
      </CardContent>
    </Card>
  )
}
