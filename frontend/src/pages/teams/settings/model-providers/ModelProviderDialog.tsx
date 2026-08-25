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
  base_url: z.url('Must be a valid URL').trim(),
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

/**
 * The provider being copied in from another team, and the team it comes from.
 * Present ⇒ the dialog is in COPY mode (#834).
 */
export interface CopySource {
  provider: ModelProviderResponse
  sourceTeamName: string
}

/**
 * The trust decision the copy actually asks the user to make (#834/#827): the
 * credential's *use* moves to a different set of members. Stated before the
 * confirm, never after.
 */
function CopyCredentialWarning({
  sourceTeamName,
}: Readonly<{ sourceTeamName: string }>) {
  return (
    <div
      className="border-warning/20 bg-warning/5 rounded-md border p-3 text-sm"
      data-testid="copy-credential-warning"
    >
      <p>
        <span className="font-medium">
          {sourceTeamName}&apos;s API key will be copied across and every member
          of this team will be able to use it.
        </span>{' '}
        The key itself is never shown here, and usage billed to it will include
        this team&apos;s requests. Only copy a provider whose credential you are
        entitled to share.
      </p>
    </div>
  )
}

/**
 * Copy mode has no key to enter: the credential moves server-side as ciphertext
 * and is never in the SPA's hands, so it is rendered read-only rather than as
 * an input the user could fill in to no effect.
 *
 * Plain Label/Input, not FormField: `useFormField` throws outside a FormField,
 * and there is no form value behind this field.
 */
function CopiedApiKeyField({
  sourceTeamName,
  hasApiKey,
}: Readonly<{ sourceTeamName: string; hasApiKey: boolean }>) {
  return (
    <div className="space-y-2 sm:col-span-2">
      <Label htmlFor="copy-api-key">API key</Label>
      <Input
        id="copy-api-key"
        readOnly
        disabled
        value={`Will be copied from ${sourceTeamName}`}
        data-testid="copy-api-key-field"
      />
      <p className="text-muted-foreground text-sm">
        {hasApiKey
          ? 'The stored key travels with the copy. You can replace it later by editing this provider.'
          : 'That provider has no key stored, so the copy will not have one either.'}
      </p>
    </div>
  )
}

interface Props {
  teamId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  provider?: ModelProviderResponse
  /**
   * Set to put the dialog in copy mode: the form is pre-filled from the source
   * team's provider, the API key is not enterable (the server carries the
   * source row's ciphertext across), and the validate-on-save probe is skipped
   * — with no key in the SPA's hands it could only fail with an auth error.
   * Mutually exclusive with `provider`, which is the edit path.
   */
  copySource?: CopySource
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
  copySource,
  submitting,
  onSubmit,
}: Readonly<Props>) {
  const [validating, setValidating] = useState(false)
  const isCopy = !!copySource

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
    // Copy mode pre-fills from the SOURCE team's provider; every field stays
    // an editable override of what the server would otherwise carry across.
    // `is_default` is forced false because the copy always lands non-default
    // server-side, so it can never displace this team's existing default.
    const prefill = copySource?.provider ?? provider
    if (prefill) {
      form.reset({
        name: prefill.name,
        provider_type: prefill.provider_type,
        model: prefill.model,
        base_url: prefill.base_url ?? '',
        api_key: '',
        is_default: copySource ? false : prefill.is_default,
      })
    }
  }, [open, provider, copySource, form])

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
    //
    // Never on the copy path (#834), and gated on the dialog's MODE rather
    // than on "the key field is empty": an empty key is also a legitimate
    // create-path error state, and conflating the two would suppress a real
    // validation failure there. The copy has no key to probe with — the
    // credential never leaves the server — so the probe could only ever fail
    // with an auth error and block a perfectly valid copy.
    if (!isCopy && (!provider || identityChanged(values))) {
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
  const editLabel = provider ? 'Save changes' : 'Add provider'
  const submitLabel = isCopy ? 'Copy provider' : editLabel

  const title = () => {
    if (isCopy) return 'Copy model provider'
    return provider ? 'Edit provider' : 'Add model provider'
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title()}</DialogTitle>
          <DialogDescription>
            {isCopy ? (
              <>
                Pre-filled from{' '}
                <span className="font-medium">{copySource.provider.name}</span>{' '}
                in{' '}
                <span className="font-medium">{copySource.sourceTeamName}</span>
                . Adjust anything you want to differ here — the copy is a
                snapshot and changing it won&apos;t affect the other team.
              </>
            ) : (
              'Model providers point VibeXP at an OpenAI-compatible LLM backend.'
            )}
          </DialogDescription>
        </DialogHeader>
        {isCopy && (
          <CopyCredentialWarning sourceTeamName={copySource.sourceTeamName} />
        )}
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
            {isCopy ? (
              <CopiedApiKeyField
                sourceTeamName={copySource.sourceTeamName}
                hasApiKey={copySource.provider.has_api_key}
              />
            ) : (
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
            )}
            {/* A copy always lands non-default server-side, so offering the
                checkbox here would be a control that silently does nothing. */}
            {!isCopy && (
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
                        Model requests without an explicit provider will use
                        this one.
                      </FormDescription>
                    </div>
                  </FormItem>
                )}
              />
            )}
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
                {validating ? 'Validating…' : submitLabel}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
