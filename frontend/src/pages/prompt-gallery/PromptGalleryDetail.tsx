import { AlertCircle, ArrowLeft, FileText, Tags, Wand2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingSection,
  useCopyAction,
} from '@/components/patterns/reading-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Panel,
  PanelBody,
  PanelHeader,
  PanelTitle,
} from '@/components/ui/panel'
import { useAlertContext } from '@/contexts/AlertContext'
import type { PromptGalleryTemplate } from '@/services/promptGalleryService'
import { promptGalleryService } from '@/services/promptGalleryService'
import { getErrorMessage } from '@/utils/errorHandling'

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
      <ReadingPage title="Loading prompt…">
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </ReadingPage>
    )
  }

  if (!prompt) {
    return (
      <ReadingPage title="Prompt not found" actions={[backAction]}>
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Prompt not found</AlertTitle>
          <AlertDescription>
            The prompt you&apos;re looking for doesn&apos;t exist or has been
            removed.
          </AlertDescription>
        </Alert>
      </ReadingPage>
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
      onClick: () => {
        void handleUsePrompt()
      },
    },
  ]

  const sections: ReadingSection[] = [
    {
      id: 'category',
      label: 'Category',
      icon: FileText,
      content: (
        <Panel>
          <PanelHeader>
            <PanelTitle>Category</PanelTitle>
          </PanelHeader>
          <PanelBody className="pb-4">
            <Badge variant="secondary" className="gap-1">
              <FileText className="size-3" />
              {prompt.category}
            </Badge>
          </PanelBody>
        </Panel>
      ),
    },
    {
      id: 'tags',
      label: 'Tags',
      icon: Tags,
      content: prompt.tags && prompt.tags.length > 0 && (
        <Panel>
          <PanelHeader>
            <PanelTitle>Tags</PanelTitle>
          </PanelHeader>
          <PanelBody className="pb-4">
            <div className="flex flex-wrap gap-1.5">
              {prompt.tags.map(tag => (
                <Badge key={tag} variant="outline">
                  {tag}
                </Badge>
              ))}
            </div>
          </PanelBody>
        </Panel>
      ),
    },
  ]

  return (
    <ReadingPage
      title={prompt.title}
      description={prompt.description}
      actions={actions}
      sections={sections}
    >
      <MarkdownRenderer content={prompt.content} syntaxTheme="auto" />
    </ReadingPage>
  )
}
