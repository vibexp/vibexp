import { AlertCircle, Bot, Shield } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import type { AgentCard } from '@/services/agentService'

import { primaryInterface } from '../helpers'

interface AgentPreviewProps {
  loading: boolean
  data: AgentCard | null
  error: string | null
  onRetry: () => void
}

export function AgentPreview({
  loading,
  data,
  error,
  onRetry,
}: Readonly<AgentPreviewProps>) {
  if (loading) {
    return (
      <Card>
        <CardContent className="space-y-3 p-6">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-32 w-full" />
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="size-4" />
        <AlertTitle>Unable to fetch agent card</AlertTitle>
        <AlertDescription className="space-y-2">
          <p>{error}</p>
          <Button size="sm" variant="outline" onClick={onRetry}>
            Retry
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  if (!data) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center justify-center py-12">
          <Bot className="text-muted-foreground mb-3 size-12" />
          <p className="text-muted-foreground text-sm">
            Enter an agent URL to see preview
          </p>
        </CardContent>
      </Card>
    )
  }

  const iface = primaryInterface(data)

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="space-y-4 p-6">
          <div>
            <h4 className="text-xl font-semibold">{data.name}</h4>
            {data.description && (
              <p className="text-muted-foreground mt-1">{data.description}</p>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <span className="text-muted-foreground">Protocol:</span>{' '}
              <span className="font-medium">
                {iface?.protocolVersion ?? 'Not specified'}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Version:</span>{' '}
              <span className="font-medium">{data.version}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Transport:</span>{' '}
              <span className="font-medium">
                {iface?.protocolBinding ?? 'Not specified'}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Streaming:</span>{' '}
              <span className="font-medium">
                {data.capabilities?.streaming ? 'Enabled' : 'Disabled'}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4 p-6">
          <h5 className="font-semibold">Input &amp; output modes</h5>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="mb-2 text-sm font-medium">Input</p>
              <div className="flex flex-wrap gap-1.5">
                {(data.defaultInputModes ?? []).map(mode => (
                  <Badge key={mode} variant="secondary">
                    {mode}
                  </Badge>
                ))}
              </div>
            </div>
            <div>
              <p className="mb-2 text-sm font-medium">Output</p>
              <div className="flex flex-wrap gap-1.5">
                {(data.defaultOutputModes ?? []).map(mode => (
                  <Badge key={mode} variant="secondary">
                    {mode}
                  </Badge>
                ))}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {(data.skills?.length ?? 0) > 0 && (
        <Card>
          <CardContent className="space-y-4 p-6">
            <h5 className="font-semibold">
              Skills &amp; capabilities ({data.skills?.length ?? 0})
            </h5>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {(data.skills ?? []).map(skill => (
                <div
                  key={skill.id ?? skill.name}
                  className="bg-muted/50 space-y-2 rounded-md border p-3"
                >
                  <div>
                    <div className="font-medium">{skill.name}</div>
                    {skill.description && (
                      <p className="text-muted-foreground mt-1 text-sm">
                        {skill.description}
                      </p>
                    )}
                  </div>
                  {(skill.tags?.length ?? 0) > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {(skill.tags ?? []).map(tag => (
                        <Badge key={tag} variant="outline">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  )}
                  {(skill.examples?.length ?? 0) > 0 && (
                    <>
                      <Separator />
                      <div>
                        <p className="mb-1 text-xs font-medium">Example</p>
                        <p className="text-muted-foreground bg-background rounded border p-2 text-xs italic">
                          &quot;{skill.examples?.[0]}&quot;
                        </p>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {data.securityRequirements &&
        Array.isArray(data.securityRequirements) &&
        data.securityRequirements.length > 0 && (
          <Card>
            <CardContent className="space-y-3 p-6">
              <h5 className="font-semibold">Security</h5>
              {/* A security requirement entry is an alternative combination of
                  scheme names; duplicate combinations are meaningless, so the
                  joined scheme names identify the entry. */}
              {data.securityRequirements.map((sec: Record<string, unknown>) => (
                <div
                  key={Object.keys(sec).join('+')}
                  className="text-muted-foreground flex items-center gap-2 text-sm"
                >
                  <Shield className="size-4" />
                  <span>
                    Authentication required: {Object.keys(sec).join(', ')}
                  </span>
                </div>
              ))}
            </CardContent>
          </Card>
        )}
    </div>
  )
}
