import { z } from 'zod'

import type { EmbeddingProviderResponse } from '@/services/embeddingProviderService'

export const schema = z.object({
  name: z.string().trim().min(1, 'Name is required').max(255),
  provider_type: z.string().min(1, 'Provider type is required'),
  model: z.string().trim().min(1, 'Model is required').max(255),
  base_url: z.url('Must be a valid URL').trim(),
  api_key: z.string().optional(),
  concurrency: z
    .number()
    .int('Must be a whole number')
    .min(1, 'Must be at least 1'),
  // Prefixes are NOT trimmed — asymmetric models expect a trailing space (e.g.
  // "query: "), which trimming would strip. Capped at 256 to match the backend.
  query_prefix: z.string().max(256, 'Must be at most 256 characters'),
  document_prefix: z.string().max(256, 'Must be at most 256 characters'),
  is_default: z.boolean(),
  // Chunk sizing is rendered (and sent) on the COPY path only — a copy carries
  // the source row's values across, and this is where they can be overridden.
  // Create/edit keep the server's defaults, as they always have.
  chunk_size: z
    .number()
    .int('Must be a whole number')
    .min(1, 'Must be at least 1'),
  chunk_overlap: z
    .number()
    .int('Must be a whole number')
    .min(0, 'Must be 0 or more'),
  // Copy-only: opts the copy into a background re-embed of this team's content.
  reprocess: z.boolean(),
})

export type EmbeddingProviderFormValues = z.infer<typeof schema>

/**
 * The provider being copied in from another team, and the team it comes from.
 * Present ⇒ the dialog is in COPY mode (#835).
 */
export interface CopySource {
  provider: EmbeddingProviderResponse
  sourceTeamName: string
}

/**
 * What the copy path hands back to the page: the (possibly edited) overrides
 * plus the `reprocess` opt-in. Deliberately not `CreateEmbeddingProviderRequest`
 * — `reprocess` exists only on the copy endpoint, and the API key never travels
 * through the SPA at all.
 */
export interface CopySubmitValues {
  name: string
  provider_type: string
  model: string
  base_url: string
  chunk_size: number
  chunk_overlap: number
  concurrency: number
  query_prefix: string
  document_prefix: string
  reprocess: boolean
}

// identityChanged is true when an edit changes the model, base URL, or provider
// type — the fields that make existing embeddings incomparable. It gates the
// validate-on-save probe. Module-level (pure) to keep the dialog under the
// max-lines-per-function cap.
export function identityChanged(
  values: EmbeddingProviderFormValues,
  provider?: EmbeddingProviderResponse
) {
  return (
    !!provider &&
    (values.model.trim() !== provider.model ||
      values.base_url.trim() !== (provider.base_url ?? '') ||
      values.provider_type !== provider.provider_type)
  )
}

// reembedWillTrigger is true when an edit will wipe + re-index this team's
// embeddings: an identity change OR a document_prefix change (it alters the text
// every document is embedded with). A query_prefix change does NOT re-index — it
// affects only the query side — so it is intentionally excluded.
export function reembedWillTrigger(
  values: EmbeddingProviderFormValues,
  provider?: EmbeddingProviderResponse
) {
  return (
    identityChanged(values, provider) ||
    (!!provider && values.document_prefix !== (provider.document_prefix ?? ''))
  )
}
