import type { Control, UseFormReturn } from 'react-hook-form'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { EmbeddingProviderFormValues } from '@/pages/teams/settings/embedding-providers/embeddingProviderForm'

/**
 * Copy mode has no key to enter: the credential moves server-side as ciphertext
 * and is never in the SPA's hands, so it is rendered read-only rather than as an
 * input the user could fill in to no effect.
 *
 * Plain Label/Input, not FormField: `useFormField` throws outside a FormField,
 * and there is no form value behind this field.
 */
export function CopiedApiKeyField({
  sourceTeamName,
  hasApiKey,
}: Readonly<{ sourceTeamName: string; hasApiKey: boolean }>) {
  return (
    <div className="space-y-2 sm:col-span-2">
      <Label htmlFor="copy-embedding-api-key">API key</Label>
      <Input
        id="copy-embedding-api-key"
        readOnly
        disabled
        value={`Will be copied from ${sourceTeamName}`}
        data-testid="copy-api-key-field"
      />
      <p className="text-sm text-muted-foreground">
        {hasApiKey
          ? 'The stored key travels with the copy. You can replace it later by editing this provider.'
          : 'That provider has no key stored, so the copy will not have one either.'}
      </p>
    </div>
  )
}

// Numeric form field with the string↔number coercion an <input type="number">
// needs. Mirrors ConcurrencyField; used for the two copy-only chunk fields.
function NumberField({
  control,
  name,
  label,
  description,
  min,
}: Readonly<{
  control: Control<EmbeddingProviderFormValues>
  name: 'chunk_size' | 'chunk_overlap'
  label: string
  description: string
  min: number
}>) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              type="number"
              min={min}
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
          <FormDescription>{description}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

/**
 * The copy-only controls: the source row's chunk sizing (pre-filled, editable)
 * and the re-embed opt-in that posts the endpoint's `reprocess` flag.
 */
export function CopyOnlyFields({
  form,
}: Readonly<{ form: UseFormReturn<EmbeddingProviderFormValues> }>) {
  return (
    <>
      <NumberField
        control={form.control}
        name="chunk_size"
        label="Chunk size"
        description="Characters per chunk when documents are embedded."
        min={1}
      />
      <NumberField
        control={form.control}
        name="chunk_overlap"
        label="Chunk overlap"
        description="Characters shared between adjacent chunks."
        min={0}
      />
      <FormField
        control={form.control}
        name="reprocess"
        render={({ field }) => (
          <FormItem className="flex flex-row items-start gap-2 space-y-0 sm:col-span-2">
            <FormControl>
              <Checkbox
                checked={field.value}
                onCheckedChange={value => {
                  field.onChange(value === true)
                }}
                className="mt-0.5"
                data-testid="copy-reprocess-checkbox"
              />
            </FormControl>
            <div className="space-y-0.5 leading-none">
              <FormLabel>Re-process embeddings after copying</FormLabel>
              <FormDescription>
                Regenerates this team&apos;s embeddings in the background so
                they match whichever provider ends up active.
              </FormDescription>
            </div>
          </FormItem>
        )}
      />
    </>
  )
}
