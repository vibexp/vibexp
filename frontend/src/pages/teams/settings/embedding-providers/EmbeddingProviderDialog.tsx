import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { type Control, useForm, type UseFormReturn } from 'react-hook-form'

import { ConfirmDialog } from '@/components/ConfirmDialog'
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
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'
import {
  CopiedApiKeyField,
  CopyOnlyFields,
} from '@/pages/teams/settings/embedding-providers/EmbeddingProviderCopyFields'
import type {
  CopySource,
  CopySubmitValues,
  EmbeddingProviderFormValues,
} from '@/pages/teams/settings/embedding-providers/embeddingProviderForm'
import {
  identityChanged,
  reembedWillTrigger,
  schema,
} from '@/pages/teams/settings/embedding-providers/embeddingProviderForm'
import type {
  CreateEmbeddingProviderRequest,
  EmbeddingProviderResponse,
  UpdateEmbeddingProviderRequest,
} from '@/services/embeddingProviderService'
import {
  EMBEDDING_VECTOR_DIMENSIONS,
  embeddingProviderService,
} from '@/services/embeddingProviderService'

// Concurrency is the one numeric field, so it needs value/onChange coercion
// (an <input type="number"> yields strings, but the schema wants a number).
// Extracted to keep the dialog component under the max-lines-per-function cap.
function ConcurrencyField({
  control,
}: Readonly<{
  control: Control<EmbeddingProviderFormValues>
}>) {
  return (
    <FormField
      control={control}
      name="concurrency"
      render={({ field }) => (
        <FormItem>
          <FormLabel>Concurrency</FormLabel>
          <FormControl>
            <Input
              type="number"
              min={1}
              step={1}
              name={field.name}
              ref={field.ref}
              onBlur={field.onBlur}
              value={Number.isNaN(field.value) ? '' : field.value}
              onChange={event => {
                field.onChange(event.target.valueAsNumber)
              }}
            />
          </FormControl>
          <FormDescription>
            Max simultaneous embedding requests to this provider; keep at 1 for
            single-threaded providers.
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

// PREFIX_PRESETS are one-click instruction prefixes for known asymmetric model
// families. Each sets BOTH sides at once (query and document), since a family
// prescribes both. "None" clears them (symmetric models / current default).
const PREFIX_PRESETS: {
  label: string
  query: string
  document: string
}[] = [
  {
    label: 'mxbai / BGE (English)',
    query: 'Represent this sentence for searching relevant passages: ',
    document: '',
  },
  { label: 'E5', query: 'query: ', document: 'passage: ' },
  { label: 'None', query: '', document: '' },
]

// PrefixFields renders the query/document instruction-prefix inputs plus the
// per-family preset chips. Extracted to keep the dialog component under the
// max-lines-per-function cap (mirrors ConcurrencyField). Preset chips are used
// instead of a Select because a Radix Select inside a Dialog crashes jsdom.
function PrefixFields({
  form,
}: Readonly<{
  form: UseFormReturn<EmbeddingProviderFormValues>
}>) {
  return (
    <div className="space-y-3 sm:col-span-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground">Presets:</span>
        {PREFIX_PRESETS.map(preset => (
          <Button
            key={preset.label}
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              form.setValue('query_prefix', preset.query, {
                shouldDirty: true,
              })
              form.setValue('document_prefix', preset.document, {
                shouldDirty: true,
              })
            }}
          >
            {preset.label}
          </Button>
        ))}
      </div>
      <FormField
        control={form.control}
        name="query_prefix"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Query prefix</FormLabel>
            <FormControl>
              <Textarea
                {...field}
                rows={2}
                placeholder="e.g., Represent this sentence for searching relevant passages: "
              />
            </FormControl>
            <FormDescription>
              Prepended to search queries before embedding. Leave empty for
              symmetric models. Changing it needs no re-index.
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="document_prefix"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Document prefix</FormLabel>
            <FormControl>
              <Textarea {...field} rows={2} placeholder="e.g., passage: " />
            </FormControl>
            <FormDescription>
              Prepended to document chunks before embedding. Changing it
              re-indexes this team&apos;s resources.
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}

// The API key input for the create/edit paths. Extracted so the dialog
// component stays under the max-lines-per-function cap.
function ApiKeyField({
  control,
  isEdit,
}: Readonly<{
  control: Control<EmbeddingProviderFormValues>
  isEdit: boolean
}>) {
  return (
    <FormField
      control={control}
      name="api_key"
      render={({ field }) => (
        <FormItem className="sm:col-span-2">
          <FormLabel>API key</FormLabel>
          <FormControl>
            <Input
              {...field}
              type="password"
              placeholder={
                isEdit ? 'Leave blank to keep current key' : 'Enter API key'
              }
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

// The "use as default" checkbox for the create/edit paths. Extracted for the
// same reason as ApiKeyField.
function DefaultProviderField({
  control,
}: Readonly<{ control: Control<EmbeddingProviderFormValues> }>) {
  return (
    <FormField
      control={control}
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
              Embedding requests without an explicit provider will use this one.
            </FormDescription>
          </div>
        </FormItem>
      )}
    />
  )
}

// Title + description for all three modes. Extracted so the dialog component
// stays under the max-lines-per-function cap.
function ProviderDialogHeader({
  provider,
  copySource,
}: Readonly<{
  provider?: EmbeddingProviderResponse
  copySource?: CopySource
}>) {
  let title = 'Add embedding provider'
  if (copySource) title = 'Copy embedding provider'
  else if (provider) title = 'Edit provider'

  return (
    <DialogHeader>
      <DialogTitle>{title}</DialogTitle>
      <DialogDescription>
        {copySource ? (
          <>
            Pre-filled from{' '}
            <span className="font-medium">{copySource.provider.name}</span> in{' '}
            <span className="font-medium">{copySource.sourceTeamName}</span>.
            Adjust anything you want to differ here — the copy is a snapshot and
            changing it won&apos;t affect the other team.
          </>
        ) : (
          'Embedding providers convert text into vectors for semantic search.'
        )}
      </DialogDescription>
    </DialogHeader>
  )
}

interface Props {
  teamId: string
  /**
   * Named in the re-embed confirmation. Before epic #536 this page always acted
   * on whichever team the header switcher had selected, so "this team" was
   * unambiguous by construction. The page is now deep-linkable, so a
   * destructive, team-wide embedding wipe can be confirmed against a team the
   * user never explicitly picked — the dialog has to say which one (#584).
   */
  teamName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  provider?: EmbeddingProviderResponse
  /**
   * Set to put the dialog in copy mode: the form is pre-filled from the source
   * team's provider, the API key is not enterable (the server carries the
   * source row's ciphertext across), and the validate-on-save probe is skipped
   * — with no key in the SPA's hands it could only fail with an auth error.
   * Mutually exclusive with `provider`, which is the edit path.
   */
  copySource?: CopySource
  submitting: boolean
  /** The create/edit submit. Not called — and not needed — in copy mode. */
  onSubmit?: (
    data: CreateEmbeddingProviderRequest | UpdateEmbeddingProviderRequest
  ) => Promise<void>
  /** Called instead of `onSubmit` in copy mode. */
  onCopySubmit?: (data: CopySubmitValues) => Promise<void>
}

export function EmbeddingProviderDialog({
  teamId,
  teamName,
  open,
  onOpenChange,
  provider,
  copySource,
  submitting,
  onSubmit,
  onCopySubmit,
}: Readonly<Props>) {
  const [validating, setValidating] = useState(false)
  const [pendingValues, setPendingValues] =
    useState<EmbeddingProviderFormValues | null>(null)
  const isCopy = !!copySource

  const form = useForm<EmbeddingProviderFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      provider_type: 'openai_compatible',
      model: '',
      base_url: '',
      api_key: '',
      concurrency: 1,
      query_prefix: '',
      document_prefix: '',
      is_default: false,
      chunk_size: 1000,
      chunk_overlap: 200,
      reprocess: false,
    },
  })

  useEffect(() => {
    if (!open) {
      form.reset()
      return
    }
    // Copy mode pre-fills from the SOURCE team's provider; every non-secret
    // field stays an editable override of what the server would otherwise carry
    // across. `is_default` is forced false because the copy always lands
    // non-default server-side, so it can never displace this team's default.
    const prefill = copySource?.provider ?? provider
    if (prefill) {
      form.reset({
        name: prefill.name,
        provider_type: prefill.provider_type,
        model: prefill.model,
        base_url: prefill.base_url ?? '',
        api_key: '',
        concurrency: prefill.concurrency,
        query_prefix: prefill.query_prefix ?? '',
        document_prefix: prefill.document_prefix ?? '',
        is_default: copySource ? false : prefill.is_default,
        chunk_size: prefill.chunk_size,
        chunk_overlap: prefill.chunk_overlap,
        reprocess: false,
      })
    }
  }, [open, provider, copySource, form])

  const proceed = async (values: EmbeddingProviderFormValues) => {
    const baseUrl = values.base_url.trim()
    const apiKey = values.api_key?.trim() ?? ''
    const model = values.model.trim()

    // The copy path never probes and never posts a key: the credential stays
    // server-side as ciphertext, so `validateEmbeddingProvider` could only ever
    // fail with an auth error and block a perfectly valid copy. Gated on the
    // dialog's MODE rather than on "the key field is empty" — an empty key is
    // also a legitimate create-path error state, and conflating the two would
    // suppress a real validation failure there.
    if (isCopy) {
      await onCopySubmit?.({
        name: values.name.trim(),
        provider_type: values.provider_type,
        model,
        base_url: baseUrl,
        chunk_size: values.chunk_size,
        chunk_overlap: values.chunk_overlap,
        concurrency: values.concurrency,
        query_prefix: values.query_prefix,
        document_prefix: values.document_prefix,
        reprocess: values.reprocess,
      })
      return
    }

    // Validate-on-save: probe the provider so it is accepted only if it returns
    // the fixed 1024-dimensional vectors VibeXP stores. Always validate on create
    // and whenever an edit changes the embedding identity; a name/default-only edit
    // needs no re-probe. When the identity changed but the provider needs a key the
    // user didn't re-enter, the probe fails with an auth error that prompts them.
    if (!provider || identityChanged(values, provider)) {
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

    await onSubmit?.({
      name: values.name.trim(),
      provider_type: values.provider_type,
      model,
      base_url: baseUrl,
      api_key: apiKey === '' ? undefined : apiKey,
      concurrency: values.concurrency,
      // Prefixes are sent verbatim (no trim — the trailing space is significant);
      // an empty string clears a previously-configured prefix on update.
      query_prefix: values.query_prefix,
      document_prefix: values.document_prefix,
      is_default: values.is_default,
    })
  }

  // Editing the model/endpoint/type or the document prefix invalidates this team's
  // existing embeddings, so confirm the wipe + re-embed before proceeding.
  const handleSubmit = form.handleSubmit(async values => {
    if (reembedWillTrigger(values, provider)) {
      setPendingValues(values)
      return
    }
    await proceed(values)
  })

  const busy = submitting || validating
  const editLabel = provider ? 'Save changes' : 'Add provider'
  const submitLabel = isCopy ? 'Copy provider' : editLabel

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-2xl">
          <ProviderDialogHeader provider={provider} copySource={copySource} />
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
                  Fixed by VibeXP&apos;s vector store. Providers are validated
                  on save to guarantee they return this width.
                </p>
              </div>
              <ConcurrencyField control={form.control} />
              <FormField
                control={form.control}
                name="base_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Base URL</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="https://api.openai.com/v1"
                      />
                    </FormControl>
                    <FormDescription>
                      Required for custom OpenAI-compatible endpoints.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {isCopy && <CopyOnlyFields form={form} />}
              <PrefixFields form={form} />
              {copySource ? (
                <CopiedApiKeyField
                  sourceTeamName={copySource.sourceTeamName}
                  hasApiKey={copySource.provider.has_api_key}
                />
              ) : (
                <ApiKeyField control={form.control} isEdit={!!provider} />
              )}
              {/* A copy always lands non-default server-side, so offering the
                  checkbox here would be a control that silently does nothing.
                  What a copy CAN still do to search is reported by the server
                  afterwards, as an activation verdict the page warns on. */}
              {!isCopy && <DefaultProviderField control={form.control} />}
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
      <ConfirmDialog
        open={pendingValues !== null}
        onOpenChange={openState => {
          if (!openState) setPendingValues(null)
        }}
        title={`Re-embed ${teamName}'s resources?`}
        description={
          <>
            Changing the model, endpoint, provider type, or document prefix
            deletes <strong>{teamName}</strong>&apos;s existing embeddings and
            re-generates them in the background. Semantic search falls back to
            keyword matching until re-indexing completes.
          </>
        }
        confirmLabel="Save & re-embed"
        variant="destructive"
        loading={busy}
        onConfirm={async () => {
          const values = pendingValues
          setPendingValues(null)
          if (values) await proceed(values)
        }}
      />
    </>
  )
}
