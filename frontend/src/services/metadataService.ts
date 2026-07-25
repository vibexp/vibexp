import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the metadata catalog (epic #519) — the OpenAPI spec
// is the single source of truth; do not hand-write request/response shapes.
export type ListMetadataKeysQuery = NonNullable<
  operations['getMetadataKeys']['parameters']['query']
>
export type ListMetadataValuesQuery = NonNullable<
  operations['getMetadataValues']['parameters']['query']
>
export type MetadataKeysResponse = components['schemas']['MetadataKeysResponse']
export type MetadataValuesResponse =
  components['schemas']['MetadataValuesResponse']

/** The resource types whose metadata the catalog can enumerate. */
export type MetadataResourceType = ListMetadataKeysQuery['resource_type']

/**
 * A metadata filter as the API takes it: key -> the values that satisfy it.
 * Keys are ANDed, values within a key ORed, and an empty array means "the key
 * exists". Serialize with `serializeMetadataFilter` before sending it as the
 * `metadata` query parameter.
 */
export type MetadataFilterValue = Record<string, string[]>

/**
 * Encodes a filter for the `metadata` query parameter on the list endpoints.
 * Returns undefined for an empty filter so callers can spread it away rather
 * than sending `metadata={}`.
 */
export function serializeMetadataFilter(
  filter: MetadataFilterValue
): string | undefined {
  if (Object.keys(filter).length === 0) return undefined
  return JSON.stringify(filter)
}

// Client-side mirror of the backend's limits (backend/paths/*.yaml), applied
// when parsing so a hand-edited URL cannot produce a request the API will 400.
const MAX_KEYS = 10
const MAX_VALUES_PER_KEY = 25
const MAX_KEY_LENGTH = 255
const MAX_VALUE_LENGTH = 512

/**
 * Decodes a `metadata` query-param value back into a filter.
 *
 * TOTAL by design: the input comes from a user-editable URL, so anything that
 * is not a JSON object of key -> array of strings (within the API's limits)
 * yields `{}` rather than throwing or forwarding garbage to the API. That
 * matches how the list pages already treat a malformed `sort_by` or date param.
 */
export function parseMetadataFilter(
  raw: string | undefined
): MetadataFilterValue {
  if (!raw) return {}

  let decoded: unknown
  try {
    decoded = JSON.parse(raw)
  } catch {
    return {}
  }

  if (
    typeof decoded !== 'object' ||
    decoded === null ||
    Array.isArray(decoded)
  ) {
    return {}
  }

  const entries = Object.entries(decoded as Record<string, unknown>)
  if (entries.length > MAX_KEYS) return {}

  const filter: MetadataFilterValue = {}
  for (const [key, value] of entries) {
    if (key === '' || key.length > MAX_KEY_LENGTH) return {}
    if (!Array.isArray(value) || value.length > MAX_VALUES_PER_KEY) return {}
    if (
      value.some(
        entry => typeof entry !== 'string' || entry.length > MAX_VALUE_LENGTH
      )
    ) {
      return {}
    }
    filter[key] = value as string[]
  }

  return filter
}

class MetadataService {
  /** Distinct metadata keys in use for a resource type, ascending. */
  async listKeys(
    teamId: string,
    query: ListMetadataKeysQuery
  ): Promise<MetadataKeysResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/metadata/keys', {
        params: { path: { team_id: teamId }, query },
      })
    )
  }

  /** Distinct values stored under one metadata key, ascending. */
  async listValues(
    teamId: string,
    query: ListMetadataValuesQuery
  ): Promise<MetadataValuesResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/{team_id}/metadata/values', {
        params: { path: { team_id: teamId }, query },
      })
    )
  }
}

export const metadataService = new MetadataService()
