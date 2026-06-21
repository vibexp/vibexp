import { ArrowLeft, Save } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { FeedForm, type FeedFormHandle } from '@/pages/feeds/FeedForm'
import { feedService } from '@/services/feedService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import type { CreateFeedRequest, UpdateFeedRequest } from '@/types/feed'

export function FeedNew() {
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()
  const [creating, setCreating] = useState(false)
  const formRef = useRef<FeedFormHandle>(null)

  const handleSubmit = async (data: CreateFeedRequest | UpdateFeedRequest) => {
    if (!currentTeam) {
      handleError(
        new Error('Team context is required'),
        'Failed to create feed'
      )
      return
    }
    try {
      setCreating(true)
      const feed = await feedService.createFeed(
        currentTeam.id,
        data as CreateFeedRequest
      )
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_CREATED,
        properties: { feed_id: feed.id, feed_name: feed.name },
      })
      showSuccess('Feed created successfully', 'Success')
      void navigate(`/feeds/${encodeURIComponent(feed.id)}`)
    } catch (error) {
      handleError(error, 'Failed to create feed')
      throw error
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Create feed"
        description="Create a new AI feed to receive content from AI assistants."
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate('/feeds')
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <Button
              onClick={() => {
                formRef.current?.submit()
              }}
              disabled={creating}
            >
              <Save className="mr-2 size-4" />
              {creating ? 'Creating…' : 'Create feed'}
            </Button>
          </>
        }
      />
      <FeedForm ref={formRef} onSubmit={handleSubmit} isLoading={creating} />
    </div>
  )
}
