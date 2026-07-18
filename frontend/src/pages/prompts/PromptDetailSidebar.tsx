import { AccessActivityPanel } from '@/components/access-activity/AccessActivityPanel'
import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import { CommentsPanel } from '@/components/comments/CommentsPanel'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import {
  MetadataPanel,
  MetaRow,
  type VersionHistoryMeta,
} from '@/components/metadata/MetadataPanel'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type {
  Prompt,
  PromptDependenciesResponse,
} from '@/services/promptService'

interface Props {
  prompt: Prompt
  teamId: string | undefined
  dependencies: PromptDependenciesResponse | null
  loadingDependencies: boolean
  versionHistory?: VersionHistoryMeta
}

export function PromptDetailSidebar({
  prompt,
  teamId,
  dependencies,
  loadingDependencies,
  versionHistory,
}: Readonly<Props>) {
  return (
    <div className="space-y-4">
      {prompt.description && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Description</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-sm">
              {prompt.description}
            </p>
          </CardContent>
        </Card>
      )}

      {prompt.labels && prompt.labels.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Labels</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-1.5">
              {prompt.labels.map(label => (
                <Badge key={label} variant="outline">
                  {label}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <MetadataPanel
        createdAt={prompt.created_at}
        updatedAt={prompt.updated_at}
        versionHistory={versionHistory}
      >
        <MetaRow label="MCP">
          {prompt.mcp_expose ? 'Exposed' : 'Not exposed'}
        </MetaRow>
      </MetadataPanel>

      {teamId && (
        <ResourceAttachments
          teamId={teamId}
          ownerType="prompt"
          ownerId={prompt.id}
        />
      )}

      {teamId && (
        <AccessActivityPanel
          teamId={teamId}
          resourceType="prompt"
          resourceId={prompt.id}
        />
      )}

      {teamId && (
        <CommentsPanel
          teamId={teamId}
          resourceType="prompt"
          resourceId={prompt.id}
        />
      )}

      {dependencies && dependencies.used_by.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Used by</CardTitle>
          </CardHeader>
          <CardContent>
            {loadingDependencies ? (
              <LoadingSpinner size="sm" />
            ) : (
              <ul className="space-y-1">
                {dependencies.used_by.map(dep => (
                  <li key={dep.slug} className="text-xs">
                    <span className="font-medium">{dep.name}</span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
