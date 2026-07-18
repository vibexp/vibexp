import { zodResolver } from '@hookform/resolvers/zod'
import type { ColumnDef } from '@tanstack/react-table'
import { ArrowUpDown, Inbox, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CopyButton } from '@/components/CopyButton'
import { DataTable } from '@/components/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'

interface Row {
  id: string
  name: string
  status: 'active' | 'draft' | 'archived'
  owner: string
  updated: string
}

const STATUS_TONE: Record<Row['status'], 'success' | 'warning' | 'neutral'> = {
  active: 'success',
  draft: 'warning',
  archived: 'neutral',
}

const SAMPLE: Row[] = [
  {
    id: '1',
    name: 'Customer onboarding',
    status: 'active',
    owner: 'Alex',
    updated: '2 days ago',
  },
  {
    id: '2',
    name: 'Weekly report',
    status: 'draft',
    owner: 'Sam',
    updated: '5 hours ago',
  },
  {
    id: '3',
    name: 'Legacy export',
    status: 'archived',
    owner: 'Jane',
    updated: '3 months ago',
  },
  {
    id: '4',
    name: 'Invoice generator',
    status: 'active',
    owner: 'Alex',
    updated: '1 day ago',
  },
  {
    id: '5',
    name: 'Quarterly review',
    status: 'draft',
    owner: 'Priya',
    updated: '30 min ago',
  },
  {
    id: '6',
    name: 'Deprecated template',
    status: 'archived',
    owner: 'Sam',
    updated: '2 years ago',
  },
  {
    id: '7',
    name: 'Welcome email',
    status: 'active',
    owner: 'Jane',
    updated: '4 hours ago',
  },
]

const formSchema = z.object({
  title: z.string().min(2, 'Title must be at least 2 characters'),
  notes: z.string().optional(),
})

type FormValues = z.infer<typeof formSchema>

// Module-level so the header/cell renderers are not re-defined per render
// (they only use module-scope values, never component state).
const columns: ColumnDef<Row>[] = [
  {
    accessorKey: 'name',
    header: ({ column }) => (
      <Button
        variant="ghost"
        size="sm"
        className="-ml-3"
        onClick={() => {
          column.toggleSorting(column.getIsSorted() === 'asc')
        }}
      >
        Name
        <ArrowUpDown className="ml-2 size-3" />
      </Button>
    ),
  },
  {
    accessorKey: 'status',
    header: 'Status',
    cell: ({ row }) => {
      const status = row.original.status
      return <StatusBadge tone={STATUS_TONE[status]}>{status}</StatusBadge>
    },
  },
  { accessorKey: 'owner', header: 'Owner' },
  { accessorKey: 'updated', header: 'Updated' },
  {
    id: 'actions',
    enableHiding: false,
    cell: ({ row }) => (
      <div className="flex justify-end gap-1">
        <CopyButton value={row.original.id} label="Copy ID" />
        <Button
          variant="ghost"
          size="icon"
          aria-label="Delete row"
          onClick={() => {
            toast.info('Pretend delete', {
              description: `Row: ${row.original.name}`,
            })
          }}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    ),
  },
]

export function Showcase() {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmLoading, setConfirmLoading] = useState(false)

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: { title: '', notes: '' },
  })

  const onSubmit = (values: FormValues) => {
    toast.success('Form submitted', { description: JSON.stringify(values) })
    form.reset()
  }

  const handleConfirm = async () => {
    setConfirmLoading(true)
    await new Promise<void>(resolve => setTimeout(resolve, 700))
    setConfirmLoading(false)
    setConfirmOpen(false)
    toast.success('Confirmed')
  }

  return (
    <div className="mx-auto max-w-6xl">
      <PageHeader
        title="v2 shell showcase"
        description="Phase 1 primitives rendering end-to-end. Parity features arrive slice by slice from Phase 2."
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => {
                toast.info('Hello from sonner')
              }}
            >
              Toast me
            </Button>
            <Button
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              New item
            </Button>
          </>
        }
      />

      <Tabs defaultValue="table" className="space-y-4">
        <TabsList>
          <TabsTrigger value="table">Data table</TabsTrigger>
          <TabsTrigger value="form">Form</TabsTrigger>
          <TabsTrigger value="states">States</TabsTrigger>
        </TabsList>

        <TabsContent value="table" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Items</CardTitle>
            </CardHeader>
            <CardContent>
              <DataTable
                columns={columns}
                data={SAMPLE}
                searchColumn="name"
                searchPlaceholder="Search by name…"
                pageSize={5}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="form">
          <Card>
            <CardHeader>
              <CardTitle>Create item</CardTitle>
            </CardHeader>
            <CardContent>
              <Form {...form}>
                <form
                  onSubmit={event => {
                    void form.handleSubmit(onSubmit)(event)
                  }}
                  className="space-y-4"
                >
                  <FormField
                    control={form.control}
                    name="title"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Title</FormLabel>
                        <FormControl>
                          <Input placeholder="Enter a title" {...field} />
                        </FormControl>
                        <FormDescription>Visible to your team.</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="notes"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Notes</FormLabel>
                        <FormControl>
                          <Textarea
                            placeholder="Optional notes"
                            rows={4}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <div className="flex gap-2">
                    <Button type="submit">Submit</Button>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => {
                        form.reset()
                      }}
                    >
                      Reset
                    </Button>
                  </div>
                </form>
              </Form>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="states" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Loading</CardTitle>
            </CardHeader>
            <CardContent>
              <LoadingSpinner label="Fetching data…" />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Empty</CardTitle>
            </CardHeader>
            <CardContent>
              <EmptyState
                icon={Inbox}
                title="Nothing here yet"
                description="Create your first item to get started."
                actions={
                  <Button>
                    <Plus className="mr-2 size-4" />
                    New item
                  </Button>
                }
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Create new item?"
        description="This is a showcase — nothing actually happens."
        confirmLabel="Create"
        loading={confirmLoading}
        onConfirm={handleConfirm}
      />
    </div>
  )
}
