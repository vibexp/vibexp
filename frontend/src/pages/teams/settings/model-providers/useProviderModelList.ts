import { useState } from 'react'
import { z } from 'zod'

import type {
  ListProviderModelsRequest,
  ModelProviderResponse,
  ProviderModel,
  ProviderModelList,
} from '@/services/modelProviderService'
import { modelProviderService } from '@/services/modelProviderService'

type ModelListFailure = NonNullable<ProviderModelList['message']>

/**
 * One fixed sentence per listing failure category (#1076). The server already
 * reduces a failure to a category and never forwards the provider's raw
 * response; this map keeps the SPA from rendering even the category verbatim.
 */
const MODEL_LIST_ERRORS: Record<ModelListFailure, string> = {
  connection_failed: "Couldn't reach the provider.",
  unauthorized: 'The provider rejected the API key.',
  misconfigured_provider:
    "The provider's response wasn't understood — check the base URL.",
  destination_not_allowed: "This base URL isn't allowed by the server.",
}

const MODEL_LIST_REQUEST_FAILED = "Couldn't load models."

/** Where the on-demand model listing stands for the dialog's current config. */
export type ModelListState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'loaded'; models: ProviderModel[] }
  | { status: 'unsupported' }
  | { status: 'error'; message: string }

const IDLE: ModelListState = { status: 'idle' }

interface Stored {
  // Bumped on every dialog open/close, so a response still in flight from a
  // previous session of the dialog can never land in the next one.
  generation: number
  // The (provider_type, base_url, api_key) the state was loaded for.
  key: string
  value: ModelListState
}

interface Options {
  open: boolean
  teamId: string
  /** The saved provider being edited, if any. */
  provider?: ModelProviderResponse
  /** Copy mode's source (#834): lists against the SOURCE team. */
  copySource?: { provider: ModelProviderResponse; sourceTeamId: string }
  providerType: string
  baseUrl: string
  apiKey: string | undefined
}

/**
 * On-demand model listing for the provider dialog (#1076).
 *
 * The state is keyed on the configuration it was loaded for, so changing the
 * provider type, base URL or API key after a load drops the list by
 * construction — no effect has to remember to clear it — while the form's
 * `model` value is left alone.
 */
export function useProviderModelList({
  open,
  teamId,
  provider,
  copySource,
  providerType,
  baseUrl,
  apiKey,
}: Options) {
  const [stored, setStored] = useState<Stored>({
    generation: 0,
    key: '',
    value: IDLE,
  })
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    setStored(prev => ({
      generation: prev.generation + 1,
      key: '',
      value: IDLE,
    }))
  }

  const trimmedBaseUrl = baseUrl.trim()
  const trimmedApiKey = apiKey?.trim() ?? ''
  const key = JSON.stringify([providerType, trimmedBaseUrl, trimmedApiKey])
  const state = stored.key === key ? stored.value : IDLE
  const canLoad =
    state.status !== 'loading' && z.url().safeParse(trimmedBaseUrl).success

  // The request per mode: create sends the typed key; edit adds the saved
  // provider so a blank key reuses the stored one; copy lists against the
  // SOURCE team with the source provider's stored key, which never enters
  // the SPA (a `provider_id` must name a provider in the path team).
  const load = async () => {
    const request: ListProviderModelsRequest = {
      provider_type: providerType,
      base_url: trimmedBaseUrl,
    }
    let listTeamId = teamId
    if (copySource) {
      listTeamId = copySource.sourceTeamId
      request.provider_id = copySource.provider.id
    } else {
      if (trimmedApiKey !== '') request.api_key = trimmedApiKey
      if (provider) request.provider_id = provider.id
    }

    const { generation } = stored
    setStored({ generation, key, value: { status: 'loading' } })
    let next: ModelListState
    try {
      const result = await modelProviderService.listProviderModels(
        listTeamId,
        request
      )
      if (result.supported) {
        next = { status: 'loaded', models: result.models }
      } else if (result.message) {
        next = { status: 'error', message: MODEL_LIST_ERRORS[result.message] }
      } else {
        next = { status: 'unsupported' }
      }
    } catch {
      next = { status: 'error', message: MODEL_LIST_REQUEST_FAILED }
    }
    setStored(prev =>
      prev.generation === generation && prev.key === key
        ? { ...prev, value: next }
        : prev
    )
  }

  return { state, canLoad, load }
}
