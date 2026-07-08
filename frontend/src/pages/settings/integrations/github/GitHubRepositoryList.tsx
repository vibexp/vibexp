import { FolderGit2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { EmptyState } from '@/components/EmptyState'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type {
  GitHubRepository,
  VisibilityFilter,
} from '@/services/githubIntegrationService'

import { GitHubRepositoryTable } from './GitHubRepositoryTable'
import { RepositoryFilters } from './RepositoryFilters'

interface GitHubRepositoryListProps {
  repositories: GitHubRepository[]
  isLoading: boolean
  totalCount: number
  hasMore?: boolean
  isLoadingMore?: boolean
  onLoadMore?: () => void
}

const REPOS_PER_PAGE = 20

export function GitHubRepositoryList({
  repositories,
  isLoading,
  totalCount,
  hasMore = false,
  isLoadingMore = false,
  onLoadMore,
}: GitHubRepositoryListProps) {
  const [currentPage, setCurrentPage] = useState(1)
  const [nameFilter, setNameFilter] = useState('')
  const [ownerFilter, setOwnerFilter] = useState('')
  const [visibilityFilter, setVisibilityFilter] =
    useState<VisibilityFilter>('all')

  const sortedRepositories = useMemo(() => {
    return [...repositories].sort((a, b) =>
      a.full_name.toLowerCase().localeCompare(b.full_name.toLowerCase())
    )
  }, [repositories])

  const filteredRepositories = useMemo(() => {
    return sortedRepositories.filter(repo => {
      if (
        nameFilter &&
        !repo.name.toLowerCase().includes(nameFilter.toLowerCase())
      ) {
        return false
      }
      if (
        ownerFilter &&
        !repo.owner.login.toLowerCase().includes(ownerFilter.toLowerCase())
      ) {
        return false
      }
      if (visibilityFilter === 'private' && !repo.private) {
        return false
      }
      if (visibilityFilter === 'public' && repo.private) {
        return false
      }
      return true
    })
  }, [sortedRepositories, nameFilter, ownerFilter, visibilityFilter])

  const uniqueOwners = useMemo(() => {
    return Array.from(
      new Set(repositories.map(repo => repo.owner.login))
    ).sort()
  }, [repositories])

  const totalPages = Math.ceil(filteredRepositories.length / REPOS_PER_PAGE)
  const startIndex = (currentPage - 1) * REPOS_PER_PAGE
  const endIndex = startIndex + REPOS_PER_PAGE
  const paginatedRepositories = filteredRepositories.slice(startIndex, endIndex)

  useEffect(() => {
    setCurrentPage(1)
  }, [nameFilter, ownerFilter, visibilityFilter])

  const handleResetFilters = () => {
    setNameFilter('')
    setOwnerFilter('')
    setVisibilityFilter('all')
    setCurrentPage(1)
  }

  const hasActiveFilters =
    nameFilter !== '' || ownerFilter !== '' || visibilityFilter !== 'all'

  if (isLoading) {
    return (
      <Card>
        <CardContent className="space-y-3 p-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-48 w-full" />
        </CardContent>
      </Card>
    )
  }

  if (repositories.length === 0) {
    return (
      <EmptyState
        icon={FolderGit2}
        title="No repositories found"
        description="No accessible repositories found in this GitHub installation."
      />
    )
  }

  return (
    <div className="space-y-4">
      <RepositoryFilters
        nameFilter={nameFilter}
        setNameFilter={setNameFilter}
        ownerFilter={ownerFilter}
        setOwnerFilter={setOwnerFilter}
        visibilityFilter={visibilityFilter}
        setVisibilityFilter={setVisibilityFilter}
        uniqueOwners={uniqueOwners}
        onResetFilters={handleResetFilters}
        hasActiveFilters={hasActiveFilters}
      />

      <div className="text-muted-foreground flex items-center justify-between text-sm">
        <span>
          Showing {paginatedRepositories.length} of{' '}
          {filteredRepositories.length} repositories
          {filteredRepositories.length !== totalCount &&
            ` (filtered from ${String(totalCount)} total)`}
        </span>
      </div>

      <GitHubRepositoryTable
        repositories={paginatedRepositories}
        currentPage={currentPage}
        totalPages={totalPages}
        onPageChange={setCurrentPage}
      />

      {hasMore && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={onLoadMore}
            disabled={isLoadingMore}
          >
            {isLoadingMore ? 'Loading…' : 'Load More'}
          </Button>
        </div>
      )}
    </div>
  )
}
