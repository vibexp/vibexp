import { AlertTriangle, ChevronDown, Github } from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import type { GitHubInstallationStatus } from '@/types/github'

interface GitHubConnectionCardProps {
  status: GitHubInstallationStatus | null
  onDisconnect: () => void
  isLoading: boolean
}

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function GitHubConnectionCard({
  status,
  onDisconnect,
  isLoading,
}: GitHubConnectionCardProps) {
  const [isExpanded, setIsExpanded] = useState(false)

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Github className="size-5" />
            GitHub Connection
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-3 w-1/2" />
        </CardContent>
      </Card>
    )
  }

  if (!status?.installed) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Github className="size-5" />
            GitHub Connection
          </CardTitle>
          <CardDescription>
            No GitHub account connected to this team workspace.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">
            Connect your GitHub account to access repositories and enable
            GitHub-powered features.
          </p>
        </CardContent>
      </Card>
    )
  }

  if (status.suspended) {
    return (
      <Alert variant="destructive">
        <AlertTriangle className="size-4" />
        <AlertTitle>GitHub Integration Suspended</AlertTitle>
        <AlertDescription>
          The GitHub App installation has been suspended. Please check your
          GitHub App installation settings and restore access.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Github className="size-5" />
          GitHub Connection
        </CardTitle>
      </CardHeader>
      <CardContent>
        <button
          type="button"
          onClick={() => {
            setIsExpanded(v => !v)
          }}
          aria-expanded={isExpanded}
          aria-controls="github-connection-details"
          className="flex w-full items-center justify-between text-left"
        >
          <span className="flex items-center gap-2 text-sm">
            <span
              className="size-2 shrink-0 rounded-full bg-success"
              data-testid="status-dot"
            />
            Connected as <strong>{status.account_login}</strong>
          </span>
          <ChevronDown
            className={`text-muted-foreground size-4 transition-transform duration-200 ${isExpanded ? 'rotate-180' : ''}`}
          />
        </button>

        {isExpanded && (
          <div id="github-connection-details" className="mt-4 space-y-3">
            <Separator />
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Installation ID</span>
              <span className="text-muted-foreground font-mono text-sm">
                {status.installation_id}
              </span>
            </div>
            {status.installed_at && (
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">Connected Since</span>
                <span className="text-muted-foreground text-sm">
                  {formatDate(status.installed_at)}
                </span>
              </div>
            )}
            <Separator />
            <div>
              <Button variant="destructive" size="sm" onClick={onDisconnect}>
                Disconnect GitHub
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
