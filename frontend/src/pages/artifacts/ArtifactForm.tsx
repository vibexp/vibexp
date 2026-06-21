import { zodResolver } from '@hookform/resolvers/zod'
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { ProjectPicker } from '@/components/ProjectPicker'
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
import { useTypes } from '@/hooks/useTypes'
import { ARTIFACT_STATUS_OPTIONS } from '@/pages/artifacts/artifactStatus'
import type {
  Artifact,
  CreateArtifactRequest,
  UpdateArtifactRequest,
} from '@/types'

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
  type: z.string().min(1, 'Type is required'),
  status: z.enum(['active', 'draft', 'archived']),
  content: z.string().min(1, 'Content is required'),
})

function slugify(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export type ArtifactFormValues = z.infer<typeof schema>

export interface ArtifactFormHandle {
  submit: () => void
}

interface ArtifactFormProps {
  artifact?: Artifact
  onSubmit: (
    data: CreateArtifactRequest | UpdateArtifactRequest
  ) => Promise<void>
  isLoading?: boolean
}

export const ArtifactForm = forwardRef<ArtifactFormHandle, ArtifactFormProps>(
  function ArtifactForm({ artifact, onSubmit, isLoading = false }, ref) {
    const formElRef = useRef<HTMLFormElement>(null)
    const slugManuallyEdited = useRef(false)
    const { types } = useTypes('artifacts')

    const form = useForm<ArtifactFormValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        title: artifact?.title ?? '',
        slug: artifact?.slug ?? '',
        description: artifact?.description ?? '',
        // The project picker searches/paginates on demand, so there is no
        // pre-loaded list to default from — the user selects one (required).
        project_id: artifact?.project_id ?? '',
        type: artifact?.type ?? 'general',
        status: artifact?.status ?? 'active',
        content: artifact?.content ?? '',
      },
    })

    useEffect(() => {
      if (artifact) {
        form.reset({
          title: artifact.title,
          slug: artifact.slug,
          description: artifact.description,
          project_id: artifact.project_id,
          type: artifact.type,
          status: artifact.status,
          content: artifact.content ?? '',
        })
      }
    }, [artifact, form])

    // On create, keep the slug in sync with the title until the user edits the
    // slug field directly (mirrors the prompt editor's behaviour).
    const titleValue = form.watch('title')
    useEffect(() => {
      if (!artifact && !slugManuallyEdited.current) {
        form.setValue('slug', slugify(titleValue), {
          shouldValidate: titleValue.length > 0,
        })
      }
    }, [titleValue, artifact, form])

    useImperativeHandle(ref, () => ({
      submit() {
        formElRef.current?.requestSubmit()
      },
    }))

    const handleSubmit = form.handleSubmit(async values => {
      await onSubmit({
        title: values.title.trim(),
        slug: values.slug.trim(),
        description: values.description?.trim() ?? '',
        project_id: values.project_id,
        type: values.type,
        status: values.status,
        content: values.content,
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
                      data-testid="artifact-content-textarea"
                      disabled={isLoading}
                      rows={22}
                      placeholder="Enter artifact content…"
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
                        <Input
                          {...field}
                          data-testid="artifact-title-input"
                          disabled={isLoading}
                        />
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
                          data-testid="artifact-slug-input"
                          disabled={isLoading || !!artifact}
                          placeholder="my-artifact"
                          onChange={e => {
                            slugManuallyEdited.current = true
                            field.onChange(e)
                          }}
                        />
                      </FormControl>
                      <FormDescription>
                        Identifier used in URLs. Cannot be changed after
                        creation.
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
                          data-testid="artifact-description-input"
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
                      <FormControl>
                        <ProjectPicker
                          value={field.value}
                          onChange={projectId => {
                            field.onChange(projectId ?? '')
                          }}
                          disabled={isLoading}
                          placeholder="Select project"
                          data-testid="artifact-project-select"
                        />
                      </FormControl>
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
                          <SelectTrigger data-testid="artifact-type-select">
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {types.map(t => (
                            <SelectItem key={t.id} value={t.slug}>
                              {t.name}
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
                  name="status"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Status</FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isLoading}
                      >
                        <FormControl>
                          <SelectTrigger data-testid="artifact-status-select">
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {ARTIFACT_STATUS_OPTIONS.map(option => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Drafts are hidden from search; archived artifacts are
                        hidden from default lists and search.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>
          </div>
        </form>
      </Form>
    )
  }
)
