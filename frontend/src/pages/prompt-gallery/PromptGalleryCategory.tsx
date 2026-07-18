import { ArrowLeft, ChevronRight, FileText, Search, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { EmptyState } from '@/components/EmptyState'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAlertContext } from '@/contexts/AlertContext'
import { cn } from '@/lib/utils'
import type { PromptGalleryTemplate } from '@/services/promptGalleryService'
import { promptGalleryService } from '@/services/promptGalleryService'
import { getErrorMessage } from '@/utils/errorHandling'

const PER_PAGE = 10

/** Collects the distinct tags across the fetched prompts, sorted alphabetically. */
function collectAvailableTags(prompts: PromptGalleryTemplate[]): string[] {
  const tagsSet = new Set<string>()
  prompts.forEach(p => {
    p.tags?.forEach(tag => tagsSet.add(tag))
  })
  return Array.from(tagsSet).sort((a, b) => a.localeCompare(b))
}

export function PromptGalleryCategory() {
  const { category } = useParams<{ category: string }>()
  const navigate = useNavigate()
  const { showAlert } = useAlertContext()

  const [prompts, setPrompts] = useState<PromptGalleryTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [availableTags, setAvailableTags] = useState<string[]>([])
  const [currentPage, setCurrentPage] = useState(1)
  const [totalPages, setTotalPages] = useState(1)
  const [totalCount, setTotalCount] = useState(0)

  useEffect(() => {
    const fetchPrompts = async () => {
      if (!category) return
      try {
        setLoading(true)
        const data = await promptGalleryService.getPrompts({
          category: decodeURIComponent(category),
          search: searchQuery || undefined,
          tags: selectedTags.length > 0 ? selectedTags : undefined,
          page: currentPage,
          limit: PER_PAGE,
        })
        setPrompts(data.prompts)
        setTotalCount(data.total_count)
        setTotalPages(data.total_pages)

        setAvailableTags(collectAvailableTags(data.prompts))
      } catch (error) {
        showAlert({
          type: 'error',
          message: getErrorMessage(error, 'Failed to load prompts'),
        })
      } finally {
        setLoading(false)
      }
    }
    void fetchPrompts()
  }, [category, searchQuery, selectedTags, currentPage, showAlert])

  const hasActiveFilters = searchQuery !== '' || selectedTags.length > 0

  const toggleTag = (tag: string) => {
    setSelectedTags(prev =>
      prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    )
    setCurrentPage(1)
  }

  const clearFilters = () => {
    setSearchQuery('')
    setSelectedTags([])
    setCurrentPage(1)
  }

  const categoryLabel = category ? decodeURIComponent(category) : 'Prompts'

  const renderPrompts = () => {
    if (prompts.length === 0) {
      return (
        <EmptyState
          icon={FileText}
          title="No prompts found"
          description={
            hasActiveFilters
              ? 'Try adjusting your filters or search terms.'
              : 'No prompts available in this category.'
          }
          actions={
            hasActiveFilters ? (
              <Button
                variant="outline"
                size="sm"
                data-testid="clear-filters-button"
                onClick={clearFilters}
              >
                Clear filters
              </Button>
            ) : null
          }
        />
      )
    }
    return (
      <>
        <div className="space-y-3">
          {prompts.map(prompt => (
            <Card
              key={prompt.id}
              role="button"
              tabIndex={0}
              data-testid="gallery-prompt-card"
              className="hover:border-primary/40 cursor-pointer transition-colors"
              onClick={() => {
                void navigate(`/prompt-gallery/prompt/${prompt.id}`)
              }}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  void navigate(`/prompt-gallery/prompt/${prompt.id}`)
                }
              }}
            >
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                <div className="flex-1 space-y-1">
                  <CardTitle className="text-base">{prompt.title}</CardTitle>
                  <CardDescription>{prompt.description}</CardDescription>
                </div>
                <ChevronRight className="text-muted-foreground size-5 shrink-0" />
              </CardHeader>
              {prompt.tags && prompt.tags.length > 0 && (
                <CardContent>
                  <div className="flex flex-wrap gap-1.5">
                    {prompt.tags.map(tag => (
                      <Badge
                        key={tag}
                        variant={
                          selectedTags.includes(tag) ? 'default' : 'outline'
                        }
                      >
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </CardContent>
              )}
            </Card>
          ))}
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between gap-2">
            <div className="text-muted-foreground text-sm">
              Page {currentPage} of {totalPages}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={currentPage <= 1}
                onClick={() => {
                  setCurrentPage(currentPage - 1)
                  window.scrollTo({ top: 0, behavior: 'smooth' })
                }}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={currentPage >= totalPages}
                onClick={() => {
                  setCurrentPage(currentPage + 1)
                  window.scrollTo({ top: 0, behavior: 'smooth' })
                }}
              >
                Next
              </Button>
            </div>
          </div>
        )}
      </>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={categoryLabel}
        description={`${String(totalCount)} ${totalCount === 1 ? 'prompt' : 'prompts'} available`}
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

      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="flex flex-wrap gap-2">
            <div className="relative min-w-[240px] flex-1">
              <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
              <Input
                value={searchQuery}
                onChange={e => {
                  setSearchQuery(e.target.value)
                  setCurrentPage(1)
                }}
                placeholder="Search prompts by title or description…"
                className="pl-8"
              />
            </div>
            {hasActiveFilters && (
              <Button
                variant="outline"
                size="sm"
                data-testid="clear-filters-button"
                onClick={clearFilters}
              >
                <X className="mr-2 size-4" />
                Clear filters
              </Button>
            )}
          </div>

          {availableTags.length > 0 && (
            <div className="space-y-2">
              <div className="text-muted-foreground text-xs font-medium">
                Tags
              </div>
              <div className="flex flex-wrap gap-1.5">
                {availableTags.map(tag => {
                  const active = selectedTags.includes(tag)
                  return (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => {
                        toggleTag(tag)
                      }}
                    >
                      <Badge
                        variant={active ? 'default' : 'outline'}
                        className={cn(
                          'cursor-pointer gap-1',
                          !active && 'hover:bg-muted'
                        )}
                      >
                        {tag}
                        {active && <X className="size-3" />}
                      </Badge>
                    </button>
                  )
                })}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {loading ? (
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>
      ) : (
        renderPrompts()
      )}
    </div>
  )
}
