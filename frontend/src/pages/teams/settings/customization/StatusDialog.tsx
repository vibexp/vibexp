import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'

import { slugify } from '@/components/patterns/resource'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import type { StatusDefinition } from '@/services/statusService'

const MAX_LENGTH = 50

// Mirrors the server's rules (`internal/services/status_management.go`): the
// name is trimmed and 1–50 characters, counted the way Go counts them (code
// points, not UTF-16 units); the slug is lowercase letters and numbers in
// hyphen-separated groups, at most 50. Reserved and duplicate slugs are the
// server's call — it answers 409 and the message lands on the slug field.
const nameSchema = z
  .string()
  .trim()
  .min(1, 'Name is required')
  .refine(
    value => Array.from(value).length <= MAX_LENGTH,
    `Name must be at most ${String(MAX_LENGTH)} characters`
  )

const createSchema = z.object({
  name: nameSchema,
  slug: z
    .string()
    .trim()
    .min(1, 'Slug is required')
    .max(MAX_LENGTH, `Slug must be at most ${String(MAX_LENGTH)} characters`)
    .regex(
      /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
      'Lowercase letters, numbers, and dashes only'
    ),
})

// A rename sends the name alone, so the slug it displays is never validated.
const renameSchema = z.object({ name: nameSchema, slug: z.string() })

export type StatusFormValues = z.infer<typeof createSchema>

/** Where a server error is shown: on a field, or above the form. */
export type StatusFormErrorTarget = 'name' | 'slug' | 'root'

interface StatusDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'rename'
  /** Singular resource type, e.g. "prompt" — "Create prompt status". */
  resourceLabel: string
  /** The status being renamed. Ignored in create mode. */
  status?: StatusDefinition | null
  submitting: boolean
  onSubmit: (
    values: StatusFormValues,
    setError: (target: StatusFormErrorTarget, message: string) => void
  ) => void | Promise<void>
}

/**
 * Create a custom status, or rename one.
 *
 * One component for both because they are the same two fields; a rename shows
 * the slug read-only, since the slug is what resources store and it cannot
 * change once created.
 */
export function StatusDialog({
  open,
  onOpenChange,
  mode,
  resourceLabel,
  status,
  submitting,
  onSubmit,
}: Readonly<StatusDialogProps>) {
  const isRename = mode === 'rename'
  const slugManuallyEdited = useRef(false)
  const form = useForm<StatusFormValues>({
    resolver: zodResolver(isRename ? renameSchema : createSchema),
    defaultValues: { name: '', slug: '' },
  })

  const initialName = isRename ? (status?.name ?? '') : ''
  const initialSlug = isRename ? (status?.slug ?? '') : ''
  useEffect(() => {
    if (open) {
      form.reset({ name: initialName, slug: initialSlug })
      slugManuallyEdited.current = false
    }
  }, [open, initialName, initialSlug, form])

  const handleSubmit = form.handleSubmit(values =>
    onSubmit(values, (target, message) => {
      form.setError(target, { message })
    })
  )

  const nameValue = useWatch({ control: form.control, name: 'name' })
  const slugValue = useWatch({ control: form.control, name: 'slug' })
  const trimmedName = nameValue.trim()
  // A rename to the same name would write an audit entry for no change.
  const canSubmit = isRename
    ? trimmedName.length > 0 && trimmedName !== initialName
    : trimmedName.length > 0 && slugValue.trim().length > 0
  const rootError = form.formState.errors.root?.message

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isRename ? 'Rename status' : `Create ${resourceLabel} status`}
          </DialogTitle>
          <DialogDescription>
            {isRename
              ? 'Change the name your team sees. The slug stays the same.'
              : `Add a status your team can give its ${resourceLabel} resources.`}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            data-testid="status-form"
            onSubmit={event => {
              void handleSubmit(event)
            }}
            className="space-y-4"
          >
            {rootError && (
              <Alert variant="destructive" data-testid="status-form-error">
                <AlertDescription>{rootError}</AlertDescription>
              </Alert>
            )}
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="status-name-input"
                      placeholder="e.g., In review"
                      {...field}
                      onChange={event => {
                        field.onChange(event)
                        // Keep the slug in step with the name until the user
                        // edits the slug directly.
                        if (!isRename && !slugManuallyEdited.current) {
                          form.setValue('slug', slugify(event.target.value), {
                            shouldValidate: form.formState.isSubmitted,
                          })
                        }
                      }}
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
                      data-testid="status-slug-input"
                      placeholder="in-review"
                      readOnly={isRename}
                      {...field}
                      onChange={event => {
                        slugManuallyEdited.current = true
                        field.onChange(event)
                      }}
                    />
                  </FormControl>
                  <FormDescription>
                    {isRename
                      ? "The value stored on resources. It can't be changed."
                      : "The value stored on resources. Lowercase letters, numbers, and dashes. It can't be changed later."}
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
                data-testid="submit-status-button"
              >
                {submitLabel(isRename, submitting)}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}

function submitLabel(isRename: boolean, submitting: boolean): string {
  if (isRename) return submitting ? 'Saving…' : 'Save'
  return submitting ? 'Creating…' : 'Create status'
}
