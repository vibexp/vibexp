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
