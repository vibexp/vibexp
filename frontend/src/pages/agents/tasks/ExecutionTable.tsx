import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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

interface ExecutionTableProps {
  executions: AgentExecution[]
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
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

export function ExecutionTable({
  executions,
  currentPage,
  totalPages,
  onPageChange,
}: ExecutionTableProps) {
  return (
    <div className="space-y-3">
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
            {executions.map(execution => (
              <TableRow key={execution.id}>
                <TableCell className="max-w-md">
                  <div className="line-clamp-2 text-sm">
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

      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2">
          <div className="text-muted-foreground text-sm">
            Page {currentPage} of {totalPages}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onPageChange(currentPage - 1)
              }}
              disabled={currentPage <= 1}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onPageChange(currentPage + 1)
              }}
              disabled={currentPage >= totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
