import { ArrowLeft, ChevronRight, FileText, Search, X } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useNavigate, useParams } from 'react-router'

import { EmptyState } from '@/components/EmptyState'
import { ListPage, listPageStatus } from '@/components/patterns/list-page'
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
import { useResourceListFilters } from '@/hooks/useResourceListFilters'
import { useResourceListQuery } from '@/hooks/useResourceListQuery'
import { cn } from '@/lib/utils'
import type { PromptGalleryTemplate } from '@/services/promptGalleryService'
import { promptGalleryService } from '@/services/promptGalleryService'
import { getErrorMessage } from '@/utils/errorHandling'

/** Every other resource list pages 20 at a time; the gallery used to page 10. */
const PER_PAGE = 20

/**
 * Filter defaults. `metadata` and `sort_order` are unused by the public gallery
 * endpoint but are part of `ResourceListBaseFilters`, so they are declared here
 * and left empty — an empty value never reaches the URL.
 */
const FILTER_DEFAULTS = {
  page: '1',
  search: '',
  metadata: '',
  sort_order: '',
  tags: '',
}

/** Collects the distinct tags across the fetched prompts, sorted alphabetically. */
function collectAvailableTags(prompts: PromptGalleryTemplate[]): string[] {
  const tagsSet = new Set<string>()
  prompts.forEach(p => {
    p.tags?.forEach(tag => tagsSet.add(tag))
  })
  return Array.from(tagsSet).sort((a, b) => a.localeCompare(b))
}

interface CategoryListProps {
  /** The raw (still percent-encoded) route segment. */
  category: string
}

/**
 * The gallery's category listing, on the shared `ListPage` (#917).
 *
 * Mounted with `key={category}` by the exported wrapper below: the filter state
 * lives in the URL through `useUrlFilters`, which captures its defaults once on
 * mount, and the search box keeps uncommitted text in component state — so a
 * category change has to remount rather than mutate anything in place.
 */
function CategoryList({ category }: Readonly<CategoryListProps>) {
  const navigate = useNavigate()
  const { showAlert } = useAlertContext()

  const {
    filters,
    setFilters,
    searchInput,
    setSearchInput,
    page,
    setPage,
    hasActiveFilters,
    handleClear,
  } = useResourceListFilters({
    defaults: FILTER_DEFAULTS,
    filterKeys: ['tags'],
    // The gallery is a public list: it belongs to no team and no project, so
    // there is no project selection to wait for or to reset the page on.
    projectId: undefined,
    isProjectLoading: false,
  })

  const tagsParam = filters.tags
  const selectedTags = useMemo(
    () => (tagsParam ? tagsParam.split(',').filter(Boolean) : []),
    [tagsParam]
  )

  const categoryLabel = decodeURIComponent(category)

  const handleError = useCallback(
    (error: unknown) => {
      showAlert({
        type: 'error',
        message: getErrorMessage(error, 'Failed to load prompts'),
      })
    },
    [showAlert]
  )

  const load = useCallback(async () => {
    const data = await promptGalleryService.getPrompts({
      category: categoryLabel,
      search: filters.search || undefined,
      tags: selectedTags.length > 0 ? selectedTags : undefined,
      page,
      limit: PER_PAGE,
    })
    return {
      items: data.prompts,
      totalPages: data.total_pages,
      total: data.total_count,
    }
    // `selectedTags` is memoized on the raw URL param, so it is referentially
    // stable and safe as a dependency of this memoized loader.
  }, [categoryLabel, filters.search, selectedTags, page])

  const state = useResourceListQuery({
    ready: category !== '',
    load,
    errorFallback: 'Failed to load prompts',
    onError: handleError,
  })

  // Preserved from the pre-#917 page: the facet is collected from the CURRENT
  // page's results, so it changes as you page. A stable facet needs a catalog
  // the gallery API does not expose.
  const availableTags = useMemo(
    () => collectAvailableTags(state.items),
    [state.items]
  )

  const toggleTag = (tag: string) => {
    const next = selectedTags.includes(tag)
      ? selectedTags.filter(t => t !== tag)
      : [...selectedTags, tag]
    setFilters({ tags: next.join(',') })
  }

  const openPrompt = (id: string) => {
    void navigate(`/prompt-gallery/prompt/${id}`)
  }

  const listStatus = listPageStatus(
    state.loading,
    state.error,
    state.items.length === 0
  )

  return (
    <ListPage>
      <ListPage.Header
        title={categoryLabel}
        description={`${String(state.total)} ${state.total === 1 ? 'prompt' : 'prompts'} available`}
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

      <ListPage.Container>
        <ListPage.Filters>
          <div className="space-y-4">
            <div className="flex flex-wrap gap-2">
              <div className="relative min-w-[240px] flex-1">
                <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
                <Input
                  value={searchInput}
                  onChange={e => {
                    setSearchInput(e.target.value)
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
                  onClick={handleClear}
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
          </div>
        </ListPage.Filters>

        <ListPage.Body
          status={listStatus}
          errorTitle="Failed to load prompts"
          errorMessage={state.error}
          empty={
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
                    onClick={handleClear}
                  >
                    Clear filters
                  </Button>
                ) : null
              }
            />
          }
        >
          {/* Cards, not a `ListTable`: the gallery has no columns worth
              sorting and the cards are its marketing surface. */}
          <div className="space-y-3 p-4">
            {state.items.map(prompt => (
              <Card
                key={prompt.id}
                role="button"
                tabIndex={0}
                data-testid="gallery-prompt-card"
                className="hover:border-primary/40 cursor-pointer transition-colors"
                onClick={() => {
                  openPrompt(prompt.id)
                }}
                onKeyDown={e => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    openPrompt(prompt.id)
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
        </ListPage.Body>

        <ListPage.Footer
          count={
            listStatus === 'loading' || listStatus === 'error'
              ? undefined
              : {
                  visible: state.items.length,
                  total: state.total,
                  noun: 'prompt',
                }
          }
          pagination={{
            page,
            totalPages: state.totalPages,
            onPageChange: setPage,
          }}
          hideCount={listStatus === 'loading'}
        />
      </ListPage.Container>
    </ListPage>
  )
}

export function PromptGalleryCategory() {
  const { category } = useParams<{ category: string }>()
  return <CategoryList key={category} category={category ?? ''} />
}
