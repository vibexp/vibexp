import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { githubIntegrationService } from '@/services/githubIntegrationService'
import type { BlueprintImportReport, GitHubRepository } from '@/types/github'

import { ImportBlueprintsModal } from './ImportBlueprintsModal'
import { ImportReportModal } from './ImportReportModal'

interface ImportBlueprintsButtonProps {
  repository: GitHubRepository
}

export function ImportBlueprintsButton({
  repository,
}: ImportBlueprintsButtonProps) {
  const { currentTeam } = useTeam()
  const { handleError } = useErrorHandler()
  const [isLoading, setIsLoading] = useState(false)
  const [showConfirmModal, setShowConfirmModal] = useState(false)
  const [showReportModal, setShowReportModal] = useState(false)
  const [importReport, setImportReport] =
    useState<BlueprintImportReport | null>(null)
  const [hasImported, setHasImported] = useState(false)

  const handleImport = async () => {
    if (!currentTeam) return

    try {
      setIsLoading(true)
      const report = await githubIntegrationService.importBlueprints(
        currentTeam.id,
        repository.id
      )

      setImportReport(report)
      setShowConfirmModal(false)

      if (report.total_successful > 0) {
        setHasImported(true)
        toast.success(
          `Successfully imported ${String(report.total_successful)} blueprint${report.total_successful > 1 ? 's' : ''} from ${repository.name}`
        )
      } else {
        toast.success('Scan completed. No blueprints were imported.')
      }

      setShowReportModal(true)
    } catch (error) {
      handleError(error, 'Failed to import blueprints')
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <>
      <Button
        size="sm"
        variant={hasImported ? 'secondary' : 'default'}
        onClick={() => {
          setShowConfirmModal(true)
        }}
        disabled={isLoading}
      >
        {hasImported ? 'Blueprints Imported' : 'Import Blueprints'}
      </Button>

      <ImportBlueprintsModal
        isOpen={showConfirmModal}
        repository={repository}
        onClose={() => {
          setShowConfirmModal(false)
        }}
        onConfirm={() => {
          void handleImport()
        }}
        isLoading={isLoading}
      />

      <ImportReportModal
        isOpen={showReportModal}
        report={importReport}
        repositoryName={repository.name}
        onClose={() => {
          setShowReportModal(false)
        }}
      />
    </>
  )
}
