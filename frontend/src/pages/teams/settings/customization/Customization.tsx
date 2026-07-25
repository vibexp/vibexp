import type { ColumnDef } from '@tanstack/react-table'
import { Plus, Shapes, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { DataTable } from '@/components/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import type { CreateTypeRequest, Type } from '@/services/typeService'
import { typeService } from '@/services/typeService'

import { CreateTypeDialog, type CreateTypeFormValues } from './CreateTypeDialog'

const ARTIFACTS_RESOURCE = 'artifacts'

function buildColumns(onDelete: (type: Type) => void): ColumnDef<Type>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <span className="font-medium">{row.original.name}</span>
      ),
    },
    {
      accessorKey: 'slug',
      header: 'Slug',
      cell: ({ row }) => (
        <code className="bg-muted rounded px-2 py-0.5 font-mono text-xs">
          {row.original.slug}
        </code>
      ),
    },
    {
      id: 'origin',
      header: 'Origin',
      cell: ({ row }) =>
        row.original.is_system ? (
          <Badge variant="secondary">Default</Badge>
        ) : (
          <Badge variant="outline">Custom</Badge>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) =>
        row.original.is_system ? null : (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon"
              data-testid="delete-type-button"
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

export function Customization() {
  const { currentTeam } = useTeam()
  const { handleError } = useErrorHandler()

  const [types, setTypes] = useState<Type[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [typeToDelete, setTypeToDelete] = useState<Type | null>(null)
  const [deleting, setDeleting] = useState(false)

  const teamId = currentTeam?.id

  const loadTypes = useCallback(async () => {
    if (!teamId) return
    try {
      setIsLoading(true)
      const result = await typeService.getTypes(teamId, ARTIFACTS_RESOURCE)
      setTypes(result)
    } catch (error) {
      handleError(error, 'Failed to load artifact types')
      setTypes([])
    } finally {
      setIsLoading(false)
    }
  }, [teamId, handleError])

  useEffect(() => {
    void loadTypes()
  }, [loadTypes])

  const handleCreate = async (
    values: CreateTypeFormValues,
    setFieldError: (field: 'name' | 'slug', message: string) => void
  ) => {
    if (!teamId) return
    try {
      setIsCreating(true)
      const request: CreateTypeRequest = {
        resource_type: ARTIFACTS_RESOURCE,
        slug: values.slug.trim(),
        name: values.name.trim(),
      }
      await typeService.createType(teamId, request)
      toast.success('Artifact type created')
      setCreateOpen(false)
      await loadTypes()
    } catch (error) {
      const errors = handleError(error, 'Failed to create artifact type')
      Object.entries(errors).forEach(([field, message]) => {
        if (field === 'name' || field === 'slug') {
          setFieldError(field, message)
        }
      })
    } finally {
      setIsCreating(false)
    }
  }

  const handleDelete = async () => {
    if (!typeToDelete || !teamId) return
    try {
      setDeleting(true)
      await typeService.deleteType(teamId, typeToDelete.id)
      toast.success('Artifact type deleted')
      await loadTypes()
    } catch (error) {
      handleError(error, 'Failed to delete artifact type')
    } finally {
      setDeleting(false)
      setTypeToDelete(null)
    }
  }

  const columns = buildColumns(setTypeToDelete)

  const renderTypes = () => {
    if (types.length === 0) {
      return (
        <EmptyState
          icon={Shapes}
          title="No artifact types"
          description="Create your first custom type to organize artifacts your way."
          actions={
            <Button
              disabled={!teamId}
              onClick={() => {
                setCreateOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Create type
            </Button>
          }
        />
      )
    }
    return (
      <Card>
        <CardContent className="p-4">
          <DataTable
            columns={columns}
            data={types}
            rowTestId={() => 'type-item'}
          />
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="Customization"
        description="Tailor VibeXP to how your team works."
      />

      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">Artifact types</h2>
            <p className="text-muted-foreground text-sm">
              Categories your team can assign to artifacts. The built-in
              defaults can&apos;t be removed.
            </p>
          </div>
          <Button
            data-testid="create-type-button"
            disabled={!teamId}
            onClick={() => {
              setCreateOpen(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create type
          </Button>
        </div>

        {isLoading ? (
          <Card>
            <CardContent className="space-y-3 p-6">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </CardContent>
          </Card>
        ) : (
          renderTypes()
        )}
      </section>

      <CreateTypeDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        submitting={isCreating}
        onSubmit={handleCreate}
      />

      <ConfirmDialog
        open={!!typeToDelete}
        onOpenChange={open => {
          if (!open) setTypeToDelete(null)
        }}
        title="Delete artifact type?"
        description={
          <>
            This will permanently delete{' '}
            <span className="font-medium">
              {typeToDelete?.name ?? 'this type'}
            </span>
            . Any artifacts using it will be moved to{' '}
            <span className="font-medium">General</span>.
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
