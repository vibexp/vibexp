import { Calendar, FileText, Loader2, Search, User, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { usePromptSearch } from '@/hooks/usePromptSearch'
import type { Prompt } from '@/services/promptService'

interface PromptTemplateLoaderProps {
  isOpen: boolean
  onClose: () => void
  onSelectPrompt: (prompt: Prompt) => void
  excludeCurrentPrompt?: string
  className?: string
}

export function PromptTemplateLoader({
  isOpen,
  onClose,
  onSelectPrompt,
  excludeCurrentPrompt,
  className = '',
}: PromptTemplateLoaderProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const searchInputRef = useRef<HTMLInputElement>(null)

  const { prompts, loading, error, searchPrompts, clearResults } =
    usePromptSearch({
      limit: 20,
      excludeCurrentPrompt,
    })

  // Focus search input when modal opens
  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      setTimeout(() => {
        searchInputRef.current?.focus()
      }, 100)
    }
  }, [isOpen])

  // Search with debouncing
  useEffect(() => {
    const timeoutId = setTimeout(() => {
      if (searchQuery.trim()) {
        void searchPrompts(searchQuery)
      } else {
        clearResults()
      }
    }, 300)

    return () => {
      clearTimeout(timeoutId)
    }
  }, [searchQuery, searchPrompts, clearResults])

  const handleClose = () => {
    setSearchQuery('')
    clearResults()
    onClose()
  }

  const handleSelectPrompt = (prompt: Prompt) => {
    onSelectPrompt(prompt)
    handleClose()
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  }

  const truncateText = (text: string, maxLength = 150) => {
    if (text.length <= maxLength) return text
    return text.substring(0, maxLength) + '...'
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="fixed inset-0 bg-black/20 backdrop-blur-sm"
        onClick={handleClose}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ' ') {
            handleClose()
          }
        }}
        role="button"
        tabIndex={0}
        aria-label="Close modal"
      ></div>
      <div
        className={`relative bg-card rounded-xl shadow-xl w-full max-w-4xl mx-4 max-h-[80vh] overflow-hidden ${className}`}
      >
        {/* Header */}
        <div className="border-b border-border p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xl font-semibold text-foreground">
              Load from Existing Prompt
            </h2>
            <button
              onClick={handleClose}
              className="text-muted-foreground hover:text-foreground transition-colors"
            >
              <X className="h-6 w-6" />
            </button>
          </div>

          {/* Search Input */}
          <div className="relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              ref={searchInputRef}
              type="text"
              placeholder="Search prompts by name or content..."
              value={searchQuery}
              onChange={e => {
                setSearchQuery(e.target.value)
              }}
              className="w-full pl-10 pr-4 py-3 border border-input rounded-lg focus:ring-2 focus:ring-ring focus:border-transparent"
            />
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {/* Loading State */}
          {loading && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-primary mr-2" />
              <span className="text-muted-foreground">
                Searching prompts...
              </span>
            </div>
          )}

          {/* Error State */}
          {error && (
            <div className="bg-destructive/10 border border-destructive/30 rounded-lg p-4 text-center">
              <p className="text-destructive">{error}</p>
            </div>
          )}

          {/* Empty Search State */}
          {!loading && !error && searchQuery && prompts.length === 0 && (
            <div className="text-center py-8">
              <FileText className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
              <h3 className="text-lg font-medium text-foreground mb-2">
                No prompts found
              </h3>
              <p className="text-muted-foreground">
                Try adjusting your search terms
              </p>
            </div>
          )}

          {/* Initial State */}
          {!loading && !error && !searchQuery && (
            <div className="text-center py-8">
              <Search className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
              <h3 className="text-lg font-medium text-foreground mb-2">
                Search for a prompt to load
              </h3>
              <p className="text-muted-foreground">
                Start typing to find prompts you can use as templates
              </p>
            </div>
          )}

          {/* Search Results */}
          {!loading && !error && prompts.length > 0 && (
            <div className="space-y-4">
              {prompts.map(prompt => (
                <div
                  key={prompt.id}
                  className="bg-muted border border-border rounded-lg p-4 hover:border-ring hover:bg-accent transition-colors cursor-pointer"
                  onClick={() => {
                    handleSelectPrompt(prompt)
                  }}
                  onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      handleSelectPrompt(prompt)
                    }
                  }}
                  role="button"
                  tabIndex={0}
                  aria-label={`Load prompt: ${prompt.name}`}
                >
                  <div className="flex items-start justify-between mb-2">
                    <div className="flex-1">
                      <h3 className="font-medium text-foreground mb-1">
                        {prompt.name}
                      </h3>
                      <div className="flex items-center space-x-4 text-sm text-muted-foreground mb-2">
                        <span className="inline-flex items-center">
                          <User className="h-3 w-3 mr-1" />
                          {prompt.slug}
                        </span>
                        <span className="inline-flex items-center">
                          <Calendar className="h-3 w-3 mr-1" />
                          {formatDate(prompt.created_at)}
                        </span>
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${
                            prompt.status === 'published'
                              ? 'bg-success text-success-foreground'
                              : 'bg-warning text-warning-foreground'
                          }`}
                        >
                          {prompt.status.charAt(0).toUpperCase() +
                            prompt.status.slice(1)}
                        </span>
                      </div>
                      <p className="text-sm text-muted-foreground line-clamp-3">
                        {truncateText(prompt.body)}
                      </p>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      className="ml-4 flex-shrink-0"
                      onClick={e => {
                        e.stopPropagation()
                        handleSelectPrompt(prompt)
                      }}
                    >
                      Load Template
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="border-t border-border p-4 bg-muted">
          <div className="flex justify-between items-center">
            <p className="text-sm text-muted-foreground">
              {prompts.length > 0 &&
                `Found ${String(prompts.length)} prompt${prompts.length === 1 ? '' : 's'}`}
            </p>
            <Button variant="outline" onClick={handleClose}>
              Cancel
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
