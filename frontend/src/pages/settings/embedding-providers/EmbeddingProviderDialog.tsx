import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
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
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/lib/toast'
import { embeddingProviderService } from '@/services/embeddingProviderService'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/types'
import { EMBEDDING_VECTOR_DIMENSIONS } from '@/types/embedding'

const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  provider_type: z.string().min(1, 'Provider type is required'),
  model: z.string().trim().min(1, 'Model is required').max(255),
  base_url: z.string().trim().url('Must be a valid URL'),
  api_key: z.string().optional(),
  is_default: z.boolean(),
})

export type EmbeddingProviderFormValues = z.infer<typeof schema>

interface Props {
  teamId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  provider?: EmbeddingProviderResponse
  submitting: boolean
  onSubmit: (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => Promise<void>
}

export function EmbeddingProviderDialog({
  teamId,
  open,
  onOpenChange,
  provider,
  submitting,
  onSubmit,
}: Props) {
  const [validating, setValidating] = useState(false)

  const form = useForm<EmbeddingProviderFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      provider_type: 'openai_compatible',
      model: '',
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
        model: provider.model,
        base_url: provider.base_url ?? '',
        api_key: '',
        is_default: provider.is_default,
      })
    }
  }, [open, provider, form])

  const handleSubmit = form.handleSubmit(async values => {
    const baseUrl = values.base_url.trim()
    const apiKey = values.api_key?.trim() ?? ''
    const model = values.model.trim()

    // Validate-on-save: probe the provider so it is accepted only if it returns
    // the fixed 1024-dimensional vectors VibeXP stores. Always validate on create
    // and whenever an edit changes the embedding identity (model / base URL /
    // provider type); a name/default-only edit needs no re-probe. When the
    // identity changed but the provider needs a key the user didn't re-enter, the
    // probe fails with an auth error that prompts them to supply it.
    const identityChanged =
      model !== provider?.model ||
      baseUrl !== (provider.base_url ?? '') ||
      values.provider_type !== provider.provider_type
    if (identityChanged) {
      setValidating(true)
      try {
        const result = await embeddingProviderService.validateEmbeddingProvider(
          teamId,
          {
            provider_type: values.provider_type,
            model,
            base_url: baseUrl,
            api_key: apiKey === '' ? undefined : apiKey,
          }
        )
        if (!result.is_valid) {
          toast.error(result.message, {
            description: result.details?.error_details,
          })
          return
        }
      } catch {
        toast.error('Could not validate the embedding provider')
        return
      } finally {
        setValidating(false)
      }
    }

    await onSubmit({
      name: values.name.trim(),
      provider_type: values.provider_type,
      model,
      base_url: baseUrl,
      api_key: apiKey === '' ? undefined : apiKey,
      is_default: values.is_default,
    })
  })

  const busy = submitting || validating

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
              name="model"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Model</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder="e.g., text-embedding-3-small"
                    />
                  </FormControl>
                  <FormDescription>
                    Must return {EMBEDDING_VECTOR_DIMENSIONS}-dimensional
                    vectors.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="space-y-2">
              <Label htmlFor="embedding-dimension">Dimension</Label>
              <Input
                id="embedding-dimension"
                value={EMBEDDING_VECTOR_DIMENSIONS}
                disabled
                readOnly
                aria-label="Embedding vector dimension"
              />
              <p className="text-sm text-muted-foreground">
                Fixed by VibeXP&apos;s vector store. Providers are validated on
                save to guarantee they return this width.
              </p>
            </div>
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
                disabled={busy}
                onClick={() => {
                  onOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={busy}>
                {busy && <Loader2 className="mr-2 size-4 animate-spin" />}
                {validating
                  ? 'Validating…'
                  : provider
                    ? 'Save changes'
                    : 'Add provider'}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
