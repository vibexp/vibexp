import { Plus, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/button'
import { FeedTabs, FeedToolbar } from '@/pages/feeds/FeedChrome'
import { FeedItemList } from '@/pages/feeds/FeedItemList'
import { FeedManageDropdown } from '@/pages/feeds/FeedManageDropdown'
import { useFeeds } from '@/pages/feeds/useFeeds'

export function Feeds() {
  const navigate = useNavigate()
  const {
    tab,
    feeds,
    projects,
    assistants,
    itemsState,
    searchInput,
    setSearchInput,
    filters,
    setFilters,
    feedToDelete,
    setFeedToDelete,
    deletingFeed,
    itemToDelete,
    setItemToDelete,
    deletingItem,
    fetchItems,
    handleTabChange,
    handleArchiveItem,
    handleUnarchiveItem,
    handleDeleteItem,
    handleDeleteFeed,
    getFeedName,
    getProjectName,
    getMember,
    hasFilters,
    activeCount,
    archivedCount,
  } = useFeeds()

  return (
    <div className="space-y-6">
      <PageHeader
        title="AI Feeds"
        description="Browse AI-generated content posted by your team's AI assistants."
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              aria-label="Refresh"
              onClick={() => {
                void fetchItems(filters)
              }}
              disabled={itemsState.loading}
            >
              <RefreshCw
                className={`size-4 ${itemsState.loading ? 'animate-spin' : ''}`}
              />
            </Button>
            <FeedManageDropdown feeds={feeds} onDeleteFeed={setFeedToDelete} />
            <Button
              onClick={() => {
                void navigate('/feeds/new')
              }}
            >
              <Plus className="mr-2 size-4" />
              New feed
            </Button>
          </>
        }
      />

      <div className="space-y-5">
        <FeedTabs
          tab={tab}
          onChange={value => {
            handleTabChange(value)
          }}
          activeCount={activeCount}
          archivedCount={archivedCount}
        />

        <FeedToolbar
          searchInput={searchInput}
          onSearchChange={setSearchInput}
          projects={projects}
          projectId={filters.project_id}
          onProjectChange={v => {
            setFilters(prev => ({ ...prev, project_id: v, page: 1 }))
          }}
          assistants={assistants}
          assistantName={filters.ai_assistant_name}
          onAssistantChange={v => {
            setFilters(prev => ({ ...prev, ai_assistant_name: v, page: 1 }))
          }}
          feeds={feeds}
          feedId={filters.feed_id}
          onFeedChange={v => {
            setFilters(prev => ({ ...prev, feed_id: v, page: 1 }))
          }}
        />

        <FeedItemList
          items={itemsState.items}
          loading={itemsState.loading}
          error={itemsState.error}
          totalPages={itemsState.totalPages}
          currentPage={itemsState.currentPage}
          tab={tab}
          hasFilters={hasFilters}
          feedName={getFeedName}
          projectName={getProjectName}
          member={getMember}
          onArchive={handleArchiveItem}
          onUnarchive={handleUnarchiveItem}
          onDelete={setItemToDelete}
          onPagePrev={() => {
            setFilters(prev => ({
              ...prev,
              page: (prev.page ?? 1) - 1,
            }))
          }}
          onPageNext={() => {
            setFilters(prev => ({
              ...prev,
              page: (prev.page ?? 1) + 1,
            }))
          }}
          showMcpHint
        />
      </div>

      <ConfirmDialog
        open={!!itemToDelete}
        onOpenChange={open => {
          if (!open) setItemToDelete(null)
        }}
        title="Delete feed item?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {itemToDelete?.title ?? 'this feed item'}
            </span>
            . This action cannot be undone.
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deletingItem}
        onConfirm={handleDeleteItem}
      />
      <ConfirmDialog
        open={!!feedToDelete}
        onOpenChange={open => {
          if (!open) setFeedToDelete(null)
        }}
        title="Delete feed?"
        description={
          <>
            This will permanently delete the feed{' '}
            <span className="font-medium">
              {feedToDelete?.name ?? 'this feed'}
            </span>{' '}
            and all of its items. This action cannot be undone.
          </>
        }
        confirmLabel="Delete feed"
        variant="destructive"
        loading={deletingFeed}
        onConfirm={handleDeleteFeed}
      />
    </div>
  )
}
