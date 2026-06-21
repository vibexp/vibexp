import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/types'

const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  provider_type: z.string().min(1, 'Provider type is required'),
  base_url: z.string().trim().url('Must be a valid URL').or(z.literal('')),
  api_key: z.string().optional(),
  is_default: z.boolean(),
})

export type EmbeddingProviderFormValues = z.infer<typeof schema>

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  provider?: EmbeddingProviderResponse
  submitting: boolean
  onSubmit: (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => Promise<void>
}

export function EmbeddingProviderDialog({
  open,
  onOpenChange,
  provider,
  submitting,
  onSubmit,
}: Props) {
  const form = useForm<EmbeddingProviderFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      provider_type: 'openai_compatible',
      base_url: '',
      api_key: '',
      is_default: false,
    },
  })

  useEffect(() => {
    if (!open) {
      form.reset()
      return
    }
    if (provider) {
      form.reset({
        name: provider.name,
        provider_type: provider.provider_type,
        base_url: provider.base_url ?? '',
        api_key: '',
        is_default: provider.is_default,
      })
    }
  }, [open, provider, form])

  const handleSubmit = form.handleSubmit(async values => {
    const baseUrl = values.base_url.trim()
    const apiKey = values.api_key?.trim() ?? ''
    await onSubmit({
      name: values.name.trim(),
      provider_type: values.provider_type,
      base_url: baseUrl === '' ? undefined : baseUrl,
      api_key: apiKey === '' ? undefined : apiKey,
      is_default: values.is_default,
    })
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {provider ? 'Edit provider' : 'Add embedding provider'}
          </DialogTitle>
          <DialogDescription>
            Embedding providers convert text into vectors for semantic search.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
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
                    <Input {...field} placeholder="e.g., OpenAI Embeddings" />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="provider_type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Provider type</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="openai_compatible">
                        OpenAI-compatible
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Base URL</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="https://api.openai.com/v1" />
                  </FormControl>
                  <FormDescription>
                    Required for custom OpenAI-compatible endpoints.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="api_key"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>API key</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="password"
                      placeholder={
                        provider
                          ? 'Leave blank to keep current key'
                          : 'Enter API key'
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="is_default"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start gap-2 space-y-0">
                  <FormControl>
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={value => {
                        field.onChange(value === true)
                      }}
                      className="mt-0.5"
                    />
                  </FormControl>
                  <div className="space-y-0.5 leading-none">
                    <FormLabel>Use as default</FormLabel>
                    <FormDescription>
                      Embedding requests without an explicit provider will use
                      this one.
                    </FormDescription>
                  </div>
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
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 size-4 animate-spin" />}
                {provider ? 'Save changes' : 'Add provider'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
