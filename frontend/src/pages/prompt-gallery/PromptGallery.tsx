import { ChevronRight, FileText, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAlertContext } from '@/contexts/AlertContext'
import type { PromptGalleryCategory } from '@/services/promptGalleryService'
import { promptGalleryService } from '@/services/promptGalleryService'
import { getErrorMessage } from '@/utils/errorHandling'

export function PromptGallery() {
  const navigate = useNavigate()
  const { showAlert } = useAlertContext()
  const [categories, setCategories] = useState<PromptGalleryCategory[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchCategories = async () => {
      try {
        setLoading(true)
        const data = await promptGalleryService.getCategories()
        setCategories(data)
      } catch (error) {
        showAlert({
          type: 'error',
          message: getErrorMessage(error, 'Failed to load categories'),
        })
      } finally {
        setLoading(false)
      }
    }
    void fetchCategories()
  }, [showAlert])

  return (
    <div className="space-y-6">
      <PageHeader
        title="Prompt Gallery"
        description="Explore pre-defined reusable prompts organized by category."
      />

      {loading ? (
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      ) : categories.length === 0 ? (
        <EmptyState
          icon={FileText}
          title="No categories available"
          description="Prompt categories will appear here once they are added."
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {categories.map(category => (
            <Card
              key={category.category}
              role="button"
              tabIndex={0}
              data-testid="gallery-category-card"
              className="hover:border-primary/40 cursor-pointer transition-colors"
              onClick={() => {
                void navigate(
                  `/prompt-gallery/${encodeURIComponent(category.category)}`
                )
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  void navigate(
                    `/prompt-gallery/${encodeURIComponent(category.category)}`
                  )
                }
              }}
            >
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Sparkles className="text-muted-foreground size-4" />
                    {category.category}
                  </CardTitle>
                  <CardDescription>
                    <Badge variant="secondary">
                      {category.count}{' '}
                      {category.count === 1 ? 'prompt' : 'prompts'}
                    </Badge>
                  </CardDescription>
                </div>
                <ChevronRight className="text-muted-foreground size-5" />
              </CardHeader>
              <CardContent />
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
