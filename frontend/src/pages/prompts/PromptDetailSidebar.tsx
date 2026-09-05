import { Link2 } from 'lucide-react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import {
  MetadataPanel,
  MetaRow,
  type VersionHistoryMeta,
} from '@/components/metadata/MetadataPanel'
import type { ReadingSection } from '@/components/patterns/reading-page'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type {
  Prompt,
  PromptDependenciesResponse,
} from '@/services/promptService'

interface PromptMetadataProps {
  prompt: Prompt
  versionHistory?: VersionHistoryMeta
}

/**
 * The prompt's Metadata section: description and labels cards, then the
 * shared `MetadataPanel`. The standard panels (attachments, activity,
 * comments, relations) come from `ResourceReadingPage`, not from here.
 */
export function PromptMetadata({
  prompt,
  versionHistory,
}: Readonly<PromptMetadataProps>) {
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
    </div>
  )
}

/**
 * The prompt-only "Used by" section (prompts that reference this one),
 * appended after the standard sections. Empty when nothing uses the prompt.
 */
export function promptUsedBySection(
  dependencies: PromptDependenciesResponse | null,
  loading: boolean
): ReadingSection[] {
  if (!dependencies || dependencies.used_by.length === 0) return []
  return [
    {
      id: 'used-by',
      label: 'Used by',
      icon: Link2,
      content: (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Used by</CardTitle>
          </CardHeader>
          <CardContent>
            {loading ? (
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
      ),
    },
  ]
}
