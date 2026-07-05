import { zodResolver } from '@hookform/resolvers/zod'
import { X } from 'lucide-react'
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { MEMORY_STATUS_OPTIONS } from '@/pages/memories/memoryStatus'
import type { Project } from '@/services/projectService'
import type { CreateMemoryRequest, Memory, UpdateMemoryRequest } from '@/types'

const schema = z.object({
  text: z.string().trim().min(1, 'Memory content is required'),
  project_id: z.string().min(1, 'Project is required'),
  status: z.enum(['active', 'draft', 'archived']),
})

type FormValues = z.infer<typeof schema>

export interface MemoryFormHandle {
  submit: () => void
}

interface MemoryFormProps {
  memory?: Memory
  projects: Project[]
  onSubmit: (data: CreateMemoryRequest | UpdateMemoryRequest) => Promise<void>
  isLoading?: boolean
}

function extractTags(meta?: Record<string, unknown>): string[] {
  const tags = meta?.tags
  if (!Array.isArray(tags)) return []
  return tags.filter((t): t is string => typeof t === 'string')
}

function extractExtras(
  meta?: Record<string, unknown>
): Record<string, unknown> {
  if (!meta) return {}
  const { tags: _tags, ...rest } = meta
  void _tags
  return rest
}

export const MemoryForm = forwardRef<MemoryFormHandle, MemoryFormProps>(
  function MemoryForm({ memory, projects, onSubmit, isLoading = false }, ref) {
    const [tags, setTags] = useState<string[]>(() =>
      extractTags(memory?.metadata)
    )
    const [tagInput, setTagInput] = useState('')
    const extrasRef = useRef<Record<string, unknown>>(
      extractExtras(memory?.metadata)
    )
    const formElRef = useRef<HTMLFormElement>(null)

    const firstProjectId = projects.length > 0 ? projects[0].id : ''
    const defaultProjectId = memory?.project_id ?? firstProjectId

    const form = useForm<FormValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        text: memory?.text ?? '',
        project_id: defaultProjectId,
        status: memory?.status ?? 'active',
      },
    })

    useEffect(() => {
      const resolvedProjectId =
        memory?.project_id ?? (projects.length > 0 ? projects[0].id : '')
      if (memory) {
        form.reset({
          text: memory.text,
          project_id: resolvedProjectId,
          status: memory.status,
        })
        setTags(extractTags(memory.metadata))
        extrasRef.current = extractExtras(memory.metadata)
      } else if (projects.length > 0 && !form.getValues('project_id')) {
        form.setValue('project_id', projects[0].id)
      }
    }, [memory, projects, form])

    useImperativeHandle(ref, () => ({
      submit() {
        formElRef.current?.requestSubmit()
      },
    }))

    const submitHandler = form.handleSubmit(async values => {
      const metadata: Record<string, unknown> = { ...extrasRef.current }
      if (tags.length > 0) metadata.tags = tags
      const request: CreateMemoryRequest = {
        project_id: values.project_id,
        text: values.text.trim(),
        status: values.status,
        metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
      }
      await onSubmit(request)
    })

    const addTagFromInput = () => {
      const raw = tagInput.trim()
      if (!raw) return
      const next = raw
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0 && !tags.includes(t))
      if (next.length === 0) {
        setTagInput('')
        return
      }
      setTags([...tags, ...next])
      setTagInput('')
    }

    const removeTag = (tag: string) => {
      setTags(tags.filter(t => t !== tag))
    }

    return (
      <Form {...form}>
        <form
          ref={formElRef}
          onSubmit={event => {
            void submitHandler(event)
          }}
          className="grid gap-6 lg:grid-cols-3"
        >
          <div className="lg:col-span-2">
            <FormField
              control={form.control}
              name="text"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="sr-only">Memory content</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      data-testid="memory-content-textarea"
                      disabled={isLoading}
                      rows={24}
                      placeholder="Enter your memory content here…

Share insights, learnings, code snippets, or any valuable information you want to remember."
                      className="font-mono text-sm"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Project</CardTitle>
              </CardHeader>
              <CardContent>
                <FormField
                  control={form.control}
                  name="project_id"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="sr-only">Project</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isLoading || projects.length === 0}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue placeholder="Select project" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {projects.map(p => (
                            <SelectItem key={p.id} value={p.id}>
                              {p.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Status</CardTitle>
              </CardHeader>
              <CardContent>
                <FormField
                  control={form.control}
                  name="status"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="sr-only">Status</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isLoading}
                      >
                        <FormControl>
                          <SelectTrigger data-testid="memory-status-select">
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {MEMORY_STATUS_OPTIONS.map(option => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Drafts are hidden from search; archived memories are
                        hidden from default lists and search.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm">Tags</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                <Input
                  value={tagInput}
                  onChange={e => {
                    setTagInput(e.target.value)
                  }}
                  onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ',') {
                      e.preventDefault()
                      addTagFromInput()
                    }
                  }}
                  onBlur={() => {
                    addTagFromInput()
                  }}
                  placeholder="Add tags (comma-separated)"
                  disabled={isLoading}
                />
                {tags.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {tags.map(tag => (
                      <Badge
                        key={tag}
                        variant="secondary"
                        className="gap-1 pr-1"
                      >
                        {tag}
                        <button
                          type="button"
                          aria-label={`Remove ${tag}`}
                          className="hover:bg-muted-foreground/20 rounded-sm p-0.5"
                          onClick={() => {
                            removeTag(tag)
                          }}
                        >
                          <X className="size-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                )}
                <p className="text-muted-foreground text-xs">
                  Press Enter or comma to add. Click × to remove.
                </p>
              </CardContent>
            </Card>
          </div>
        </form>
      </Form>
    )
  }
)
