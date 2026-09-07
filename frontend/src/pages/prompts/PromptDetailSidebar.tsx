import { Link2 } from 'lucide-react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import type { ReadingSection } from '@/components/patterns/reading-page'
import {
  type ProjectRef,
  ResourceMetadataSection,
  resourceRegistry,
} from '@/components/patterns/resource'
import { Badge } from '@/components/ui/badge'
import {
  Panel,
  PanelBody,
  PanelHeader,
  PanelTitle,
} from '@/components/ui/panel'
import type {
  Prompt,
  PromptDependenciesResponse,
} from '@/services/promptService'

interface PromptMetadataProps {
  prompt: Prompt
  versionHistory?: VersionHistoryMeta
  /** Resolved owning project; omitted/null while it loads. */
  project?: ProjectRef | null
  /** Builds the Project row's link target. */
  projectHref?: (project: ProjectRef) => string
}

/**
 * The prompt's Metadata section: description and labels cards, then the
 * descriptor-generated `ResourceMetadataSection` (#903). The standard panels
 * (attachments, activity, comments, relations) come from
 * `ResourceReadingPage`, not from here.
 */
export function PromptMetadata({
  prompt,
  versionHistory,
  project,
  projectHref,
}: Readonly<PromptMetadataProps>) {
  return (
    <div className="space-y-5">
      {prompt.description && (
        <Panel>
          <PanelHeader>
            <PanelTitle>Description</PanelTitle>
          </PanelHeader>
          <PanelBody className="pb-4">
            <p className="text-muted-foreground text-sm">
              {prompt.description}
            </p>
          </PanelBody>
        </Panel>
      )}

      {prompt.labels && prompt.labels.length > 0 && (
        <Panel>
          <PanelHeader>
            <PanelTitle>Labels</PanelTitle>
          </PanelHeader>
          <PanelBody className="pb-4">
            <div className="flex flex-wrap gap-1.5">
              {prompt.labels.map(label => (
                <Badge key={label} variant="outline">
                  {label}
                </Badge>
              ))}
            </div>
          </PanelBody>
        </Panel>
      )}

      <ResourceMetadataSection
        descriptor={resourceRegistry.prompt}
        resource={prompt}
        versionHistory={versionHistory}
        project={project}
        projectHref={projectHref}
      />
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
        <Panel>
          <PanelHeader>
            <PanelTitle>Used by</PanelTitle>
          </PanelHeader>
          <PanelBody className="pb-4">
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
          </PanelBody>
        </Panel>
      ),
    },
  ]
}
