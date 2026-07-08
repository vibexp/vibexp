import { ExternalLink } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import type { GitHubRepository } from '@/services/githubIntegrationService'
import { githubIntegrationService } from '@/services/githubIntegrationService'

import { ImportProjectModal } from './ImportProjectModal'

interface ImportProjectButtonProps {
  repository: GitHubRepository
}

export function ImportProjectButton({ repository }: ImportProjectButtonProps) {
  const { currentTeam } = useTeam()
  const { handleError } = useErrorHandler()
  const [isLoading, setIsLoading] = useState(false)
  const [showModal, setShowModal] = useState(false)
  const [importedSlugFromSession, setImportedSlugFromSession] = useState<
    string | null
  >(null)

  const importedSlug =
    importedSlugFromSession ?? repository.imported_project_slug ?? null

  const handleImport = async () => {
    if (!currentTeam) return

    try {
      setIsLoading(true)
      const result = await githubIntegrationService.importProject(
        currentTeam.id,
        String(repository.id)
      )

      setImportedSlugFromSession(result.project.slug)
      setShowModal(false)

      if (result.created) {
        toast.success(`Project "${result.project.name}" created successfully`)
      } else {
        toast.success(
          result.message ?? 'Project already exists for this repository'
        )
      }
    } catch (error) {
      handleError(error, 'Failed to import project')
    } finally {
      setIsLoading(false)
    }
  }

  if (importedSlug) {
    return (
      <Button asChild size="sm" variant="secondary">
        <Link to={`/settings/projects/${importedSlug}`}>
          <ExternalLink className="mr-2 h-4 w-4" />
          View Project
        </Link>
      </Button>
    )
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          setShowModal(true)
        }}
        disabled={isLoading}
      >
        Import as Project
      </Button>

      <ImportProjectModal
        isOpen={showModal}
        repository={repository}
        onClose={() => {
          setShowModal(false)
        }}
        onConfirm={() => {
          void handleImport()
        }}
        isLoading={isLoading}
      />
    </>
  )
}
