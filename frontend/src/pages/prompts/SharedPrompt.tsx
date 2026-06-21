import { AlertCircle, Check, Copy, Share2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Helmet } from 'react-helmet-async'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { BRAND_LOGO_URL, SITE_NAME, SITE_URL } from '@/config/siteConfig'
import { useAnalytics } from '@/hooks/useAnalytics'
import { promptShareService } from '@/services/promptShareService'
import type { SharedPromptResponse } from '@/types'
import { ANALYTICS_EVENTS } from '@/types/analytics'

export function SharedPrompt() {
  const { trackEvent } = useAnalytics()
  const { token } = useParams<{ token: string }>()
  const navigate = useNavigate()
  const [data, setData] = useState<SharedPromptResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!token) return
    const loadSharedPrompt = async (shareToken: string) => {
      setLoading(true)
      setError(null)
      try {
        const response = await promptShareService.getSharedPrompt(shareToken)
        const promptData: SharedPromptResponse =
          'data' in response
            ? (response as { data: SharedPromptResponse }).data
            : response
        setData(promptData)
        trackEvent({
          event: ANALYTICS_EVENTS.SHARED_PROMPT_VIEWED,
          properties: {
            share_token: shareToken,
            share_type: promptData.share_type,
            prompt_id: promptData.prompt.id,
            prompt_title: promptData.prompt.name,
            has_expiration: !!promptData.expires_at,
            referrer: document.referrer,
            action_context: 'view',
          },
        })
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Failed to load shared prompt'
        setError(message)
        trackEvent({
          event: ANALYTICS_EVENTS.SHARED_PROMPT_ERROR,
          properties: {
            share_token: shareToken,
            error_message: message,
            action_context: 'error',
          },
        })
      } finally {
        setLoading(false)
      }
    }
    void loadSharedPrompt(token)
  }, [token, trackEvent])

  const copyToClipboard = () => {
    if (!data) return
    void navigator.clipboard.writeText(data.rendered_body)
    setCopied(true)
    setTimeout(() => {
      setCopied(false)
    }, 2000)
    trackEvent({
      event: ANALYTICS_EVENTS.SHARED_PROMPT_COPY_CLICKED,
      properties: {
        share_token: token,
        share_type: data.share_type,
        prompt_id: data.prompt.id,
        prompt_title: data.prompt.name,
        action_context: 'copy',
      },
    })
  }

  if (loading) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center">
        <LoadingSpinner size="lg" label="Loading shared prompt…" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="flex flex-col gap-4 p-6">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertTitle>Unable to load prompt</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
            <Button
              onClick={() => {
                void navigate('/')
              }}
            >
              Go to homepage
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!data) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="flex flex-col gap-4 p-6">
            <Alert>
              <AlertCircle className="size-4" />
              <AlertTitle>Prompt not found</AlertTitle>
              <AlertDescription>
                This shared prompt does not exist or has been removed.
              </AlertDescription>
            </Alert>
            <Button
              onClick={() => {
                void navigate('/')
              }}
            >
              Go to homepage
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  const pageTitle = `${data.prompt.name} | Shared Prompt | ${SITE_NAME}`
  const description = `${data.prompt.description || 'Shared prompt'}. Prompt shared on ${SITE_NAME}.`
  const pageUrl = window.location.href
  const imageUrl = BRAND_LOGO_URL
  const brandLink = `${SITE_URL}?utm_source=prompt_shared_page`

  return (
    <div className="bg-background min-h-screen">
      <Helmet>
        <title>{pageTitle}</title>
        <meta name="description" content={description} />
        <meta property="og:type" content="website" />
        <meta property="og:title" content={data.prompt.name} />
        <meta property="og:description" content={description} />
        <meta property="og:url" content={pageUrl} />
        <meta property="og:site_name" content={SITE_NAME} />
        <meta property="og:image" content={imageUrl} />
        <meta name="twitter:card" content="summary_large_image" />
        <meta name="twitter:title" content={data.prompt.name} />
        <meta name="twitter:description" content={description} />
        <meta name="twitter:image" content={imageUrl} />
      </Helmet>

      <header className="bg-background border-b">
        <div className="mx-auto flex max-w-4xl items-center justify-between px-4 py-4">
          <a
            href={brandLink}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-3 hover:opacity-80"
          >
            <img src="/logo_rounded.png" alt={SITE_NAME} className="h-10 w-10" />
            <div>
              <div className="text-lg font-semibold">{SITE_NAME}</div>
              <div className="text-muted-foreground text-xs">
                AI-powered prompt management
              </div>
            </div>
          </a>
          <a href={brandLink} target="_blank" rel="noopener noreferrer">
            <Button
              size="sm"
              onClick={() => {
                trackEvent({
                  event: ANALYTICS_EVENTS.SHARED_PROMPT_CTA_CLICKED,
                  properties: {
                    share_token: token,
                    share_type: data.share_type,
                    prompt_id: data.prompt.id,
                    prompt_title: data.prompt.name,
                    action_context: 'cta_click',
                  },
                })
              }}
            >
              Build prompts
            </Button>
          </a>
        </div>
      </header>

      <main className="mx-auto max-w-4xl px-4 py-8">
        <Card>
          <CardHeader>
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 space-y-2">
                <div className="text-muted-foreground flex items-center gap-2 text-sm">
                  <Share2 className="size-4" />
                  <span>Shared prompt</span>
                  {data.share_type === 'restricted' && (
                    <Badge variant="secondary">Restricted</Badge>
                  )}
                  {data.share_type === 'public' && (
                    <Badge variant="secondary">Public</Badge>
                  )}
                </div>
                <CardTitle className="text-2xl">{data.prompt.name}</CardTitle>
                {data.prompt.description && (
                  <p className="text-muted-foreground text-sm">
                    {data.prompt.description}
                  </p>
                )}
              </div>
              <Button variant="outline" size="sm" onClick={copyToClipboard}>
                {copied ? (
                  <>
                    <Check className="mr-2 size-4" />
                    Copied
                  </>
                ) : (
                  <>
                    <Copy className="mr-2 size-4" />
                    Copy
                  </>
                )}
              </Button>
            </div>
            {data.expires_at && (
              <Alert className="mt-4">
                <AlertTitle>Note</AlertTitle>
                <AlertDescription>
                  This share link expires on{' '}
                  {new Date(data.expires_at).toLocaleString()}
                </AlertDescription>
              </Alert>
            )}
          </CardHeader>
          <CardContent>
            <MarkdownRenderer content={data.rendered_body} syntaxTheme="auto" />
          </CardContent>
        </Card>
      </main>
    </div>
  )
}
