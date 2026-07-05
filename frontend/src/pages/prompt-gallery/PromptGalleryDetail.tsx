import { AlertCircle, ArrowLeft, FileText, Wand2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { CopyButton } from '@/components/CopyButton'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading prompt…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  if (!prompt) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Prompt not found"
          actions={
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/prompt-gallery')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
          }
        />
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Prompt not found</AlertTitle>
          <AlertDescription>
            The prompt you&apos;re looking for doesn&apos;t exist or has been
            removed.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={prompt.title}
        description={prompt.description}
        actions={
          <>
            <Button variant="outline" onClick={handleBack}>
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <CopyButton
              value={prompt.content}
              label="Copy content"
              size="default"
              variant="outline"
              testId="copy-button"
            />
            <Button
              onClick={() => {
                void handleUsePrompt()
              }}
            >
              <Wand2 className="mr-2 size-4" />
              Use this prompt
            </Button>
          </>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>Content</CardTitle>
            </CardHeader>
            <CardContent>
              <MarkdownRenderer content={prompt.content} syntaxTheme="auto" />
            </CardContent>
          </Card>
        </div>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Category</CardTitle>
            </CardHeader>
            <CardContent>
              <Badge variant="secondary" className="gap-1">
                <FileText className="size-3" />
                {prompt.category}
              </Badge>
            </CardContent>
          </Card>

          {prompt.tags && prompt.tags.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Tags</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-1.5">
                  {prompt.tags.map(tag => (
                    <Badge key={tag} variant="outline">
                      {tag}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
