import { ArrowLeft, Save } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { FeedForm, type FeedFormHandle } from '@/pages/feeds/FeedForm'
import type {
  CreateFeedRequest,
  Feed,
  UpdateFeedRequest,
} from '@/services/feedService'
import { feedService } from '@/services/feedService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

export function FeedEdit() {
  const { feedId } = useParams<{ feedId: string }>()
  const navigate = useNavigate()
  const { currentTeam } = useTeam()
  const { showSuccess } = useAlerts()
  const { handleError } = useErrorHandler()
  const { trackEvent } = useAnalytics()

  const [feed, setFeed] = useState<Feed | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const formRef = useRef<FeedFormHandle>(null)

  useEffect(() => {
    const loadFeed = async () => {
      if (!feedId || !currentTeam) return
      try {
        setLoading(true)
        const f = await feedService.getFeed(currentTeam.id, feedId)
        setFeed(f)
      } catch (error) {
        handleError(error, 'Failed to load feed')
      } finally {
        setLoading(false)
      }
    }
    void loadFeed()
  }, [feedId, currentTeam, handleError])

  const handleSubmit = async (data: CreateFeedRequest | UpdateFeedRequest) => {
    if (!currentTeam || !feedId) {
      handleError(
        new Error('Team context is required'),
        'Failed to update feed'
      )
      return
    }
    try {
      setSaving(true)
      const updated = await feedService.updateFeed(currentTeam.id, feedId, data)
      trackEvent({
        event: ANALYTICS_EVENTS.FEED_UPDATED,
        properties: { feed_id: updated.id, feed_name: updated.name },
      })
      showSuccess('Feed updated successfully', 'Success')
      void navigate(`/feeds/${encodeURIComponent(feedId)}`)
    } catch (error) {
      handleError(error, 'Failed to update feed')
      throw error
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Edit feed" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={feed ? `Edit: ${feed.name}` : 'Edit feed'}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                void navigate(
                  feedId ? `/feeds/${encodeURIComponent(feedId)}` : '/feeds'
                )
              }}
            >
              <ArrowLeft className="mr-2 size-4" />
              Back
            </Button>
            <Button
              onClick={() => {
                formRef.current?.submit()
              }}
              disabled={saving}
            >
              <Save className="mr-2 size-4" />
              {saving ? 'Saving…' : 'Save changes'}
            </Button>
          </>
        }
      />
      <FeedForm
        ref={formRef}
        feed={feed ?? undefined}
        onSubmit={handleSubmit}
        isLoading={saving}
      />
    </div>
  )
}
