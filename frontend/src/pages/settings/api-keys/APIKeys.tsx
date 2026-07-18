import type { ColumnDef } from '@tanstack/react-table'
import {
  Key as KeyIcon,
  Network,
  Plus,
  Sparkles,
  Terminal,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CopyButton } from '@/components/CopyButton'
import { DataTable } from '@/components/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAnalytics } from '@/hooks'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import type { APIKey, CreateAPIKeyRequest } from '@/services/apiKeyService'
import { apiKeyService } from '@/services/apiKeyService'
import { ANALYTICS_EVENTS } from '@/types/analytics'

import {
  CreateAPIKeyDialog,
  type CreateAPIKeyFormValues,
} from './CreateAPIKeyDialog'

const INTEGRATION_META: Partial<
  Record<
    string,
    { name: string; icon: React.ComponentType<{ className?: string }> }
  >
> = {
  ai_tools: { name: 'AI Tools', icon: Sparkles },
  cli: { name: 'CLI', icon: Terminal },
  mcp_server: { name: 'MCP Server', icon: Network },
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function IntegrationBadges({ apiKey }: Readonly<{ apiKey: APIKey }>) {
  if (apiKey.integrations.length === 0) {
    return (
      <span className="text-muted-foreground text-xs italic">
        Legacy key (no integrations)
      </span>
    )
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {apiKey.integrations.map(code => {
        const meta = INTEGRATION_META[code]
        if (!meta) {
          return (
            <Badge key={code} variant="outline">
              {code}
            </Badge>
          )
        }
        const Icon = meta.icon
        return (
          <Badge key={code} variant="secondary" className="gap-1">
            <Icon className="size-3" />
            {meta.name}
          </Badge>
        )
      })}
      {apiKey.is_legacy && <StatusBadge tone="warning">Legacy</StatusBadge>}
    </div>
  )
}

function buildColumns(onDelete: (apiKey: APIKey) => void): ColumnDef<APIKey>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <KeyIcon className="text-muted-foreground size-4 shrink-0" />
          <span className="font-medium">{row.original.name}</span>
        </div>
      ),
    },
    {
      accessorKey: 'key_prefix',
      header: 'Key',
      cell: ({ row }) => (
        <code
          data-testid="masked-api-key"
          className="bg-muted rounded px-2 py-0.5 font-mono text-xs"
        >
          {row.original.key_prefix}***
        </code>
      ),
    },
    {
      id: 'integrations',
      header: 'Integrations',
      cell: ({ row }) => <IntegrationBadges apiKey={row.original} />,
    },
    {
      accessorKey: 'created_at',
      header: 'Created',
      cell: ({ row }) => (
        <span
          data-testid="api-key-created-date"
          className="text-muted-foreground text-sm"
        >
          {formatDate(row.original.created_at)}
        </span>
      ),
    },
    {
      accessorKey: 'last_used_at',
      header: 'Last used',
      cell: ({ row }) =>
        row.original.last_used_at ? (
          <span className="text-muted-foreground text-sm">
            {formatDate(row.original.last_used_at)}
          </span>
        ) : (
          <span className="text-muted-foreground text-sm italic">Never</span>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <Button
            variant="ghost"
            size="icon"
            data-testid="delete-api-key-button"
            aria-label={`Delete ${row.original.name}`}
            onClick={() => {
              onDelete(row.original)
            }}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ),
    },
  ]
}

export function APIKeys() {
  const { trackEvent } = useAnalytics()
  const { handleError } = useErrorHandler()

  const [apiKeys, setApiKeys] = useState<APIKey[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const [keyToDelete, setKeyToDelete] = useState<APIKey | null>(null)
  const [deleting, setDeleting] = useState(false)

  const loadAPIKeys = useCallback(async () => {
    try {
      setIsLoading(true)
      const keys = await apiKeyService.getAPIKeys()
      setApiKeys(Array.isArray(keys) ? keys : [])
    } catch (error) {
      handleError(error, 'Failed to load API keys')
      setApiKeys([])
    } finally {
      setIsLoading(false)
    }
  }, [handleError])

  useEffect(() => {
    void loadAPIKeys()
    trackEvent({
      event: ANALYTICS_EVENTS.API_KEYS_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent, loadAPIKeys])

  const handleCreate = async (
    values: CreateAPIKeyFormValues,
    setFieldError: (field: 'name' | 'integrations', message: string) => void
  ) => {
    try {
      setIsCreating(true)
      const request: CreateAPIKeyRequest = {
        name: values.name.trim(),
        integration_codes: values.integrations,
      }
      const response = await apiKeyService.createAPIKey(request)
      trackEvent({
        event: ANALYTICS_EVENTS.API_KEY_CREATED,
        properties: {
          api_key_id: response.api_key.id,
          api_key_name: request.name,
          integration_codes: request.integration_codes,
          action_context: 'create',
        },
      })
      setCreatedKey(response.full_key)
      setCreateOpen(false)
      await loadAPIKeys()
    } catch (error) {
      const errors = handleError(error, 'Failed to create API key')
      Object.entries(errors).forEach(([field, message]) => {
        if (field === 'name' || field === 'integrations') {
          setFieldError(field, message)
        }
      })
    } finally {
      setIsCreating(false)
    }
  }

  const handleDelete = async () => {
    if (!keyToDelete) return
    try {
      setDeleting(true)
      await apiKeyService.deleteAPIKey(keyToDelete.id)
      trackEvent({
        event: ANALYTICS_EVENTS.API_KEY_DELETED,
        properties: { api_key_id: keyToDelete.id, action_context: 'delete' },
      })
      toast.success('API key deleted')
      await loadAPIKeys()
    } catch (error) {
      handleError(error, 'Failed to delete API key')
    } finally {
      setDeleting(false)
      setKeyToDelete(null)
    }
  }

  const columns = buildColumns(setKeyToDelete)

  const keysContent =
    apiKeys.length === 0 ? (
      <EmptyState
        icon={KeyIcon}
        title="No API keys yet"
        description="Create your first API key to start using the platform programmatically."
        actions={
          <Button
            onClick={() => {
              setCreateOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create your first API key
          </Button>
        }
      />
    ) : (
      <Card>
        <CardContent className="p-4">
          <DataTable
            columns={columns}
            data={apiKeys}
            rowTestId={() => 'api-key-item'}
          />
        </CardContent>
      </Card>
    )

  return (
    <div className="space-y-6">
      <PageHeader
        title="API Keys"
        description="Create and manage API keys for programmatic access to your account."
        actions={
          <Button
            data-testid="create-api-key-button"
            onClick={() => {
              setCreateOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create API key
          </Button>
        }
      />

      {createdKey && (
        <Alert data-testid="created-api-key-card">
          <KeyIcon className="size-4" />
          <AlertTitle>API key created</AlertTitle>
          <AlertDescription>
            <p className="mb-3">
              Make sure to copy your API key now — you won&apos;t be able to see
              it again.
            </p>
            <div className="bg-muted flex items-center gap-2 rounded-md p-2">
              <code
                data-testid="api-key-display"
                className="text-foreground flex-1 break-all font-mono text-xs"
              >
                {createdKey}
              </code>
              <CopyButton value={createdKey} testId="copy-api-key-button" />
              <Button
                variant="ghost"
                size="sm"
                data-testid="close-api-key-modal-button"
                onClick={() => {
                  setCreatedKey(null)
                }}
              >
                Close
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : (
        keysContent
      )}

      <CreateAPIKeyDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        submitting={isCreating}
        onSubmit={handleCreate}
      />

      <ConfirmDialog
        open={!!keyToDelete}
        onOpenChange={open => {
          if (!open) setKeyToDelete(null)
        }}
        title="Delete API key?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {keyToDelete?.name ?? 'this key'}
            </span>
            {'. Any integrations using this key will stop working.'}
          </>
        }
        confirmLabel="Delete"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </div>
  )
}
