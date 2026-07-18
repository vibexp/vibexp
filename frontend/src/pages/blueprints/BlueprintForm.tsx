import { zodResolver } from '@hookform/resolvers/zod'
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { MetadataEditor } from '@/components/metadata/MetadataEditor'
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
import type {
  Blueprint,
  CreateBlueprintRequest,
  UpdateBlueprintRequest,
} from '@/services/blueprintService'
import type { Project } from '@/services/projectService'

const schema = z.object({
  title: z.string().trim().min(1, 'Title is required').max(255),
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .max(255)
    .regex(/^[a-z0-9-]+$/, 'Lowercase letters, numbers, and dashes only'),
  description: z.string().max(500).optional(),
  project_id: z.string().min(1, 'Project is required'),
  type: z.enum(['general', 'claude-code', 'claude', 'cursor', 'codex']),
  content: z.string().min(1, 'Content is required'),
})

export type BlueprintFormValues = z.infer<typeof schema>

// Stable reference so the editor doesn't re-validate on every render. Sub-agents
// blueprints require a `model` metadata key (enforced in the backend at
// internal/services/blueprint.go); marking it required keeps the row from being
// deleted or blanked in the UI.
const SUB_AGENTS_REQUIRED_KEYS = ['model']

export interface BlueprintFormHandle {
  submit: () => void
}

interface BlueprintFormProps {
  blueprint?: Blueprint
  projects: Project[]
  onSubmit: (
    data: CreateBlueprintRequest | UpdateBlueprintRequest
  ) => Promise<void>
  isLoading?: boolean
}

export const BlueprintForm = forwardRef<
  BlueprintFormHandle,
  BlueprintFormProps
>(function BlueprintForm(
  { blueprint, projects, onSubmit, isLoading = false },
  ref
) {
  const formElRef = useRef<HTMLFormElement>(null)
  const [metadata, setMetadata] = useState<Record<string, unknown>>(
    blueprint?.metadata ?? {}
  )
  const [metadataValid, setMetadataValid] = useState(true)

  const requiredMetadataKeys =
    blueprint?.subtype === 'sub-agents' ? SUB_AGENTS_REQUIRED_KEYS : undefined

  const form = useForm<BlueprintFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      title: blueprint?.title ?? '',
      slug: blueprint?.slug ?? '',
      description: blueprint?.description ?? '',
      project_id: blueprint?.project_id ?? '',
      type: blueprint?.type ?? 'general',
      content: blueprint?.content ?? '',
    },
  })

  useEffect(() => {
    if (blueprint) {
      form.reset({
        title: blueprint.title,
        slug: blueprint.slug,
        description: blueprint.description,
        project_id: blueprint.project_id,
        type: blueprint.type,
        content: blueprint.content,
      })
      setMetadata(blueprint.metadata ?? {})
    }
  }, [blueprint, form])

  useImperativeHandle(ref, () => ({
    submit() {
      formElRef.current?.requestSubmit()
    },
  }))

  const handleSubmit = form.handleSubmit(async values => {
    // The editor surfaces its own inline errors; block the submit so an invalid
    // metadata map (e.g. a blanked required `model` key) never reaches the API.
    if (!metadataValid) return
    await onSubmit({
      title: values.title.trim(),
      slug: values.slug.trim(),
      description: values.description?.trim() ?? '',
      project_id: values.project_id,
      type: values.type,
      content: values.content,
      metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
    })
  })

  return (
    <Form {...form}>
      <form
        ref={formElRef}
        onSubmit={event => {
          void handleSubmit(event)
        }}
        className="grid gap-6 lg:grid-cols-3"
      >
        <div className="space-y-4 lg:col-span-2">
          <FormField
            control={form.control}
            name="content"
            render={({ field }) => (
              <FormItem>
                <FormLabel className="sr-only">Content</FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    disabled={isLoading}
                    rows={22}
                    placeholder="Enter blueprint content…"
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
              <CardTitle className="text-sm">Details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <FormField
                control={form.control}
                name="title"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Title</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={isLoading} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="slug"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Slug</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        disabled={isLoading || !!blueprint}
                        placeholder="my-blueprint"
                      />
                    </FormControl>
                    <FormDescription>
                      Identifier used in URLs. Cannot be changed after creation.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Description</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        disabled={isLoading}
                        rows={3}
                        placeholder="Short description (optional)"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="project_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Project</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      disabled={isLoading}
                    >
                      <FormControl>
                        <SelectTrigger data-testid="blueprint-project-select">
                          <SelectValue placeholder="Select project" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {projects.map(project => (
                          <SelectItem key={project.id} value={project.id}>
                            {project.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="type"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Type</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      disabled={isLoading}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value="general">General</SelectItem>
                        <SelectItem value="claude-code">Claude Code</SelectItem>
                        <SelectItem value="claude">Claude</SelectItem>
                        <SelectItem value="cursor">Cursor</SelectItem>
                        <SelectItem value="codex">Codex</SelectItem>
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
              <CardTitle className="text-sm">Metadata</CardTitle>
            </CardHeader>
            <CardContent>
              <MetadataEditor
                value={metadata}
                onChange={setMetadata}
                onValidityChange={setMetadataValid}
                requiredKeys={requiredMetadataKeys}
                disabled={isLoading}
              />
            </CardContent>
          </Card>
        </div>
      </form>
    </Form>
  )
})
