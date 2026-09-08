import { Link2 } from 'lucide-react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import type { ReadingSection } from '@/components/patterns/reading-page'
import {
  type ProjectRef,
  ResourceMetadataSection,
  resourceRegistry,
  ResourceTaxonomySection,
} from '@/components/patterns/resource'
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
  projectHref?: (project: ProjectRef) => string | null
}

/**
 * The prompt's details column: the descriptor-generated metadata rows (#903)
 * followed by the unified taxonomy section (#904). The summary lives in the
 * reading header since #902 and labels are taxonomy, so this owns no panels of
 * its own; the standard panels (attachments, activity, comments, relations)
 * come from `ResourceReadingPage`.
 */
export function PromptMetadata({
  prompt,
  versionHistory,
  project,
  projectHref,
}: Readonly<PromptMetadataProps>) {
  return (
    <div className="space-y-5">
      <ResourceMetadataSection
        descriptor={resourceRegistry.prompt}
        resource={prompt}
        versionHistory={versionHistory}
        project={project}
        projectHref={projectHref}
      />

      <ResourceTaxonomySection
        descriptor={resourceRegistry.prompt}
        resource={prompt}
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
