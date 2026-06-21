import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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

const schema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .max(255, 'Name must be under 255 characters'),
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .max(255, 'Slug must be under 255 characters')
    .regex(
      /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
      'Lowercase letters, numbers, and dashes only'
    ),
})

export type CreateTypeFormValues = z.infer<typeof schema>

function slugify(value: string): string {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

interface CreateTypeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  submitting: boolean
  onSubmit: (
    values: CreateTypeFormValues,
    setFieldError: (field: 'name' | 'slug', message: string) => void
  ) => void | Promise<void>
}

export function CreateTypeDialog({
  open,
  onOpenChange,
  submitting,
  onSubmit,
}: CreateTypeDialogProps) {
  const slugManuallyEdited = useRef(false)
  const form = useForm<CreateTypeFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', slug: '' },
  })

  useEffect(() => {
    if (!open) {
      form.reset()
      slugManuallyEdited.current = false
    }
  }, [open, form])

  // Keep the slug in sync with the name until the user edits it directly.
  const nameValue = form.watch('name')
  useEffect(() => {
    if (!slugManuallyEdited.current) {
      form.setValue('slug', slugify(nameValue), {
        shouldValidate: nameValue.length > 0,
      })
    }
  }, [nameValue, form])

  const handleSubmit = form.handleSubmit(values => {
    return onSubmit(values, (field, message) => {
      form.setError(field, { message })
    })
  })

  const slugValue = form.watch('slug')
  const canSubmit = nameValue.trim().length > 0 && slugValue.trim().length > 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create artifact type</DialogTitle>
          <DialogDescription>
            Add a custom category your team can assign to artifacts.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            data-testid="create-type-form"
            onSubmit={event => {
              void handleSubmit(event)
            }}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="type-name-input"
                      placeholder="e.g., Bug report"
                      {...field}
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
                      data-testid="type-slug-input"
                      placeholder="bug-report"
                      {...field}
                      onChange={e => {
                        slugManuallyEdited.current = true
                        field.onChange(e)
                      }}
                    />
                  </FormControl>
                  <FormDescription>
                    Stored identifier. Lowercase letters, numbers, and dashes.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={submitting}
                onClick={() => {
                  onOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={submitting || !canSubmit}
                data-testid="submit-create-type-button"
              >
                {submitting ? 'Creating…' : 'Create type'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
