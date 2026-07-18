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
import type {
  CreateModelProviderRequest,
  ModelProviderResponse,
  UpdateModelProviderRequest,
} from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'

const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  provider_type: z.string().min(1, 'Provider type is required'),
  model: z.string().trim().min(1, 'Model is required').max(255),
  base_url: z.string().trim().url('Must be a valid URL'),
  api_key: z.string().optional(),
  is_default: z.boolean(),
})

export type ModelProviderFormValues = z.infer<typeof schema>

// Convenience presets for common OpenAI-compatible endpoints. Selecting one
// prefills Base URL; the field stays editable so custom/self-hosted endpoints
// still work. Kept small and user-editable so it never drifts far from reality.
const BASE_URL_PRESETS: { label: string; base_url: string }[] = [
  { label: 'OpenAI', base_url: 'https://api.openai.com/v1' },
  { label: 'Groq', base_url: 'https://api.groq.com/openai/v1' },
  { label: 'Together', base_url: 'https://api.together.xyz/v1' },
  { label: 'OpenRouter', base_url: 'https://openrouter.ai/api/v1' },
  { label: 'Local (Ollama)', base_url: 'http://localhost:11434/v1' },
]

interface Props {
  teamId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  provider?: ModelProviderResponse
  submitting: boolean
  onSubmit: (
    data: CreateModelProviderRequest | UpdateModelProviderRequest
  ) => Promise<void>
}

export function ModelProviderDialog({
  teamId,
  open,
  onOpenChange,
  provider,
  submitting,
  onSubmit,
}: Readonly<Props>) {
  const [validating, setValidating] = useState(false)

  const form = useForm<ModelProviderFormValues>({
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

  // identityChanged is true when an edit changes the model, base URL, or
  // provider type — the fields that make the stored config point at a different
  // backend. It gates the validate-on-save probe: a name/default-only edit
  // needs no re-probe (and the user hasn't re-entered the API key anyway).
  const identityChanged = (values: ModelProviderFormValues) =>
    !!provider &&
    (values.model.trim() !== provider.model ||
      values.base_url.trim() !== (provider.base_url ?? '') ||
      values.provider_type !== provider.provider_type)

  const handleSubmit = form.handleSubmit(async values => {
    const baseUrl = values.base_url.trim()
    const apiKey = values.api_key?.trim() ?? ''
    const model = values.model.trim()

    // Validate-on-save: probe the provider so an unreachable/misconfigured
    // backend is caught before it is stored. Always validate on create and
    // whenever an edit changes the provider identity; block submit on failure.
    if (!provider || identityChanged(values)) {
      setValidating(true)
      try {
        const result = await modelProviderService.validateModelProvider(
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
        toast.error('Could not validate the model provider')
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
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {provider ? 'Edit provider' : 'Add model provider'}
          </DialogTitle>
          <DialogDescription>
            Model providers point VibeXP at an OpenAI-compatible LLM backend.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={event => {
              void handleSubmit(event)
            }}
            className="grid grid-cols-1 gap-4 sm:grid-cols-2"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="e.g., OpenAI GPT-4o" />
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
            <div className="space-y-2 sm:col-span-2">
              <Label>Presets</Label>
              <div className="flex flex-wrap gap-2">
                {BASE_URL_PRESETS.map(preset => (
                  <Button
                    key={preset.label}
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      form.setValue('base_url', preset.base_url, {
                        shouldValidate: true,
                        shouldDirty: true,
                      })
                    }}
                  >
                    {preset.label}
                  </Button>
                ))}
              </div>
              <p className="text-muted-foreground text-sm">
                Optional — prefills the Base URL below, which stays editable.
              </p>
            </div>
            <FormField
              control={form.control}
              name="model"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Model</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder="e.g., gpt-4o-mini" />
                  </FormControl>
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
                    The OpenAI-compatible endpoint base URL.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="api_key"
              render={({ field }) => (
                <FormItem className="sm:col-span-2">
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
                <FormItem className="flex flex-row items-start gap-2 space-y-0 sm:col-span-2">
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
                      Model requests without an explicit provider will use this
                      one.
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />
            <DialogFooter className="sm:col-span-2">
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
