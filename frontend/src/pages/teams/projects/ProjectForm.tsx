import { zodResolver } from '@hookform/resolvers/zod'
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

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
import { Textarea } from '@/components/ui/textarea'
import type {
  CreateProjectRequest,
  Project,
  UpdateProjectRequest,
} from '@/services/projectService'

const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .max(255)
    .regex(/^[a-z0-9-]+$/, 'Lowercase letters, numbers, and dashes only'),
  description: z.string().max(500).optional(),
  git_url: z.url('Must be a valid URL').trim().optional().or(z.literal('')),
  homepage: z.url('Must be a valid URL').trim().optional().or(z.literal('')),
})

export type ProjectFormValues = z.infer<typeof schema>

export interface ProjectFormHandle {
  submit: () => void
}

interface ProjectFormProps {
  project?: Project
  onSubmit: (data: CreateProjectRequest | UpdateProjectRequest) => Promise<void>
  isLoading?: boolean
}

export const ProjectForm = forwardRef<ProjectFormHandle, ProjectFormProps>(
  function ProjectForm({ project, onSubmit, isLoading = false }, ref) {
    const formElRef = useRef<HTMLFormElement>(null)

    const form = useForm<ProjectFormValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        name: project?.name ?? '',
        slug: project?.slug ?? '',
        description: project?.description ?? '',
        git_url: project?.git_url ?? '',
        homepage: project?.homepage ?? '',
      },
    })

    useEffect(() => {
      if (project) {
        form.reset({
          name: project.name,
          slug: project.slug,
          description: project.description,
          git_url: project.git_url,
          homepage: project.homepage,
        })
      }
    }, [project, form])

    useImperativeHandle(ref, () => ({
      submit() {
        formElRef.current?.requestSubmit()
      },
    }))

    const handleSubmit = form.handleSubmit(async values => {
      await onSubmit({
        name: values.name.trim(),
        slug: values.slug.trim(),
        description: values.description?.trim() ?? '',
        git_url: values.git_url?.trim() ?? '',
        homepage: values.homepage?.trim() ?? '',
      })
    })

    return (
      <Form {...form}>
        <form
          ref={formElRef}
          onSubmit={event => {
            void handleSubmit(event)
          }}
          className="max-w-2xl space-y-4"
        >
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Name</FormLabel>
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
                    disabled={isLoading || !!project}
                    placeholder="my-project"
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
            name="git_url"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Git URL</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    disabled={isLoading}
                    placeholder="https://github.com/user/repo"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="homepage"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Homepage</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    disabled={isLoading}
                    placeholder="https://example.com"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    )
  }
)
