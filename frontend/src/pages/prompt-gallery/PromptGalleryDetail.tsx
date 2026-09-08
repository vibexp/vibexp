import { AlertCircle, ArrowLeft, Wand2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import {
  type ReadingAction,
  ResourceBody,
  useCopyAction,
} from '@/components/patterns/reading-page'
import {
  getResourceDescriptor,
  ResourceMetadataSection,
  ResourceTaxonomySection,
} from '@/components/patterns/resource'
import { ResourceReadingPage } from '@/components/resource-detail/ResourceReadingPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { useAlertContext } from '@/contexts/AlertContext'
import type { PromptGalleryTemplate } from '@/services/promptGalleryService'
import { promptGalleryService } from '@/services/promptGalleryService'
import { getErrorMessage } from '@/utils/errorHandling'

const GALLERY_PROMPT = getResourceDescriptor('gallery-prompt')

export function PromptGalleryDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { showAlert } = useAlertContext()
  const [prompt, setPrompt] = useState<PromptGalleryTemplate | null>(null)
  const [loading, setLoading] = useState(true)
  const copyAction = useCopyAction(prompt?.content ?? '', {
    testId: 'copy-button',
  })

  useEffect(() => {
    const fetchPrompt = async () => {
      if (!id) return
      try {
        setLoading(true)
        const data = await promptGalleryService.getPromptById(id)
        setPrompt(data)
      } catch (error) {
        showAlert({
          type: 'error',
          message: getErrorMessage(error, 'Failed to load prompt'),
        })
        setTimeout(() => {
          void navigate('/prompt-gallery')
        }, 2000)
      } finally {
        setLoading(false)
      }
    }
    void fetchPrompt()
  }, [id, showAlert, navigate])

  const handleUsePrompt = async () => {
    if (!prompt) return
    try {
      await promptGalleryService.trackPromptUsage(prompt.id)
      // PromptEditor still lives in v1 until Slice 5b lands.
      void navigate('/prompts/new', {
        state: {
          title: prompt.title,
          body: prompt.content,
          description: prompt.description,
        },
      })
    } catch (error) {
      showAlert({
        type: 'error',
        message: getErrorMessage(error, 'Failed to use prompt'),
      })
    }
  }

  const handleBack = () => {
    if (prompt?.category) {
      void navigate(`/prompt-gallery/${encodeURIComponent(prompt.category)}`)
    } else {
      void navigate('/prompt-gallery')
    }
  }

  const backAction: ReadingAction = {
    id: 'back',
    label: 'Back',
    icon: ArrowLeft,
    onClick: handleBack,
  }

  if (loading) {
    return (
      <ResourceReadingPage title="Loading prompt…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ResourceReadingPage>
    )
  }

  if (!prompt) {
    return (
      <ResourceReadingPage title="Prompt not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Prompt not found</AlertTitle>
          <AlertDescription>
            The prompt you&apos;re looking for doesn&apos;t exist or has been
            removed.
          </AlertDescription>
        </Alert>
      </ResourceReadingPage>
    )
  }

  const actions: ReadingAction[] = [
    backAction,
    copyAction,
    {
      id: 'use',
      label: 'Use this prompt',
      icon: Wand2,
      emphasis: 'primary',
      // The longest label in the rail; at half width it clips.
      span: 'full',
      onClick: () => {
        void handleUsePrompt()
      },
    },
  ]

  return (
    <ResourceReadingPage
      title={prompt.title}
      updatedAt={prompt.updated_at}
      summary={prompt.description}
      actions={actions}
      // A gallery prompt is served by the public gallery API and has no
      // team-scoped resource id, so it passes no `resource`: Attachments,
      // Access activity, Comments and Relations all drop out on their own.
      metadata={
        <div className="space-y-5">
          <ResourceMetadataSection
            descriptor={GALLERY_PROMPT}
            resource={prompt}
          />
          {/* Category and Tags are both `taxonomy` fields on the descriptor,
              so they render as one "Labels & metadata" block rather than the
              two hand-built panels this page used to compose. */}
          <ResourceTaxonomySection
            descriptor={GALLERY_PROMPT}
            resource={prompt}
          />
        </div>
      }
    >
      <ResourceBody content={prompt.content} />
    </ResourceReadingPage>
  )
}
