import { Filter, Lock, RefreshCw, Search, Unlock, X } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import type { VisibilityFilter } from '@/services/githubIntegrationService'

interface RepositoryFiltersProps {
  ownerFilter: string
  setOwnerFilter: (value: string) => void
  nameFilter: string
  setNameFilter: (value: string) => void
  visibilityFilter: VisibilityFilter
  setVisibilityFilter: (value: VisibilityFilter) => void
  uniqueOwners: string[]
  onResetFilters: () => void
  hasActiveFilters: boolean
}

const ALL_OWNERS_VALUE = '__all__'

export function RepositoryFilters({
  ownerFilter,
  setOwnerFilter,
  nameFilter,
  setNameFilter,
  visibilityFilter,
  setVisibilityFilter,
  uniqueOwners,
  onResetFilters,
  hasActiveFilters,
}: RepositoryFiltersProps) {
  return (
    <Card>
      <CardContent className="space-y-4 p-4">
        <div className="flex flex-col gap-3 lg:flex-row">
          <div className="relative flex-1">
            <Search className="text-muted-foreground absolute left-2.5 top-1/2 size-4 -translate-y-1/2" />
            <Input
              type="text"
              value={nameFilter}
              onChange={e => {
                setNameFilter(e.target.value)
              }}
              placeholder="Search repositories by name…"
              aria-label="Search repositories by name"
              className="pl-8"
            />
          </div>

          <div className="lg:w-64">
            <Select
              value={ownerFilter === '' ? ALL_OWNERS_VALUE : ownerFilter}
              onValueChange={value => {
                setOwnerFilter(value === ALL_OWNERS_VALUE ? '' : value)
              }}
            >
              <SelectTrigger aria-label="Filter by repository owner">
                <SelectValue placeholder="All owners" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL_OWNERS_VALUE}>All owners</SelectItem>
                {uniqueOwners.map(owner => (
                  <SelectItem key={owner} value={owner}>
                    {owner}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="lg:w-48">
            <Select
              value={visibilityFilter}
              onValueChange={value => {
                setVisibilityFilter(value as VisibilityFilter)
              }}
            >
              <SelectTrigger aria-label="Filter by visibility">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All repositories</SelectItem>
                <SelectItem value="private">Private only</SelectItem>
                <SelectItem value="public">Public only</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {hasActiveFilters && (
            <Button variant="outline" onClick={onResetFilters}>
              <RefreshCw className="mr-2 size-4" />
              Reset
            </Button>
          )}
        </div>

        {hasActiveFilters && (
          <>
            <Separator />
            <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
              <Filter className="size-3" />
              <span className="font-medium">Active filters:</span>
              {nameFilter && (
                <Badge variant="secondary" className="gap-1">
                  Name: {nameFilter}
                  <button
                    type="button"
                    onClick={() => {
                      setNameFilter('')
                    }}
                    aria-label="Clear name filter"
                    className="hover:text-foreground"
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              )}
              {ownerFilter && (
                <Badge variant="secondary" className="gap-1">
                  Owner: {ownerFilter}
                  <button
                    type="button"
                    onClick={() => {
                      setOwnerFilter('')
                    }}
                    aria-label="Clear owner filter"
                    className="hover:text-foreground"
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              )}
              {visibilityFilter !== 'all' && (
                <Badge variant="secondary" className="gap-1">
                  {visibilityFilter === 'private' ? (
                    <>
                      <Lock className="size-3" /> Private
                    </>
                  ) : (
                    <>
                      <Unlock className="size-3" /> Public
                    </>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      setVisibilityFilter('all')
                    }}
                    aria-label="Clear visibility filter"
                    className="hover:text-foreground"
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
