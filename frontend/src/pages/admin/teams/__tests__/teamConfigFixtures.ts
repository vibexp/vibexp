import { screen } from '@testing-library/react'

import type {
  AdminEmbeddingProvider,
  AdminModelProvider,
  AdminTeamAISummaryConfig,
  AdminTeamArtifactTypes,
  AdminTeamEmailProviderConfig,
  AdminTeamEmbeddingProvidersConfig,
  AdminTeamFreshnessConfig,
  AdminTeamGitHubConfig,
  AdminTeamSearchConfig,
  AdminTeamSettingsAuditListResponse,
} from '@/services/adminService'

/**
 * Shared fixtures for the admin team configuration tab tests (#1142).
 *
 * `withSentinels` adds secret-shaped keys a server regression might leak. The
 * tabs render field by field, so none of them may reach the DOM.
 */
export const SENTINEL = 'SENTINEL-LEAK'

export function withSentinels<T extends object>(value: T): T {
  return {
    ...value,
    api_key: SENTINEL,
    secret: SENTINEL,
    last_error: SENTINEL,
    webhook_url: SENTINEL,
    smtp_username: SENTINEL,
    configuration: { token: SENTINEL },
  }
}

/** No sentinel text anywhere in the rendered document. */
export function expectNoSentinel() {
  expect(document.body.textContent).not.toContain(SENTINEL)
}

const EDITABLE_ROLES = [
  'textbox',
  'spinbutton',
  'checkbox',
  'switch',
  'combobox',
  'radio',
] as const

/** No editable control, and at most `buttons` buttons (paging only). */
export function expectReadOnly(buttons = 0) {
  for (const role of EDITABLE_ROLES) {
    expect(screen.queryAllByRole(role)).toHaveLength(0)
  }
  expect(screen.queryAllByRole('button')).toHaveLength(buttons)
  expect(document.querySelector('input, select, textarea')).toBeNull()
}

export const searchConfig = (
  overrides: Partial<AdminTeamSearchConfig> = {}
): AdminTeamSearchConfig => ({
  source: 'team',
  values: {
    recency_ranking_enabled: true,
    rank_weight_relevance: 0.7,
    rank_weight_created: 0.1,
    rank_weight_updated: 0.2,
    rank_half_life_days: 30,
  },
  instance_defaults: {
    recency_ranking_enabled: false,
    rank_weight_relevance: 0.9,
    rank_weight_created: 0.05,
    rank_weight_updated: 0.05,
    rank_half_life_days: 90,
  },
  rank_candidate_cap: 200,
  ...overrides,
})

export const aiSummaryConfig = (
  overrides: Partial<AdminTeamAISummaryConfig> = {}
): AdminTeamAISummaryConfig => ({
  source: 'team',
  values: {
    enabled: true,
    model_provider_id: 'mp-1',
    top_n: 5,
    style: 'detailed',
    max_output_tokens: 800,
  },
  instance_defaults: {
    enabled: false,
    model_provider_id: null,
    top_n: 3,
    style: 'concise',
    max_output_tokens: 400,
  },
  model_provider_name: 'OpenAI prod',
  max_top_n: 10,
  max_output_tokens_ceiling: 2000,
  available: true,
  ...overrides,
})

export const freshnessConfig = (
  overrides: Partial<AdminTeamFreshnessConfig> = {}
): AdminTeamFreshnessConfig => ({
  source: 'team',
  values: { interval_seconds: 86400, reversibility_enabled: true },
  defaults: { interval_seconds: 86400, reversibility_enabled: false },
  rules: [
    {
      id: 'r1',
      project_id: null,
      resource_types: ['artifact'],
      mediums: [],
      threshold_days: 90,
      enabled: true,
      created_at: '2026-09-01T00:00:00Z',
      updated_at: '2026-09-01T00:00:00Z',
    },
    {
      id: 'r2',
      project_id: '3f2a9c1e-0000-4000-8000-000000000001',
      resource_types: ['prompt', 'memory'],
      mediums: ['cli'],
      threshold_days: 1,
      enabled: false,
      created_at: '2026-09-02T00:00:00Z',
      updated_at: '2026-09-02T00:00:00Z',
    },
  ],
  ...overrides,
})

export const modelProvider = (
  overrides: Partial<AdminModelProvider> = {}
): AdminModelProvider => ({
  id: 'mp-1',
  name: 'OpenAI prod',
  provider_type: 'openai',
  model: 'gpt-5',
  base_url: 'https://api.openai.com/v1',
  is_default: true,
  has_api_key: true,
  configuration_keys: ['organization', 'temperature'],
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  ...overrides,
})

export const embeddingProvider = (
  overrides: Partial<AdminEmbeddingProvider> = {}
): AdminEmbeddingProvider => ({
  id: 'ep-1',
  name: 'Local embeddings',
  provider_type: 'ollama',
  model: 'nomic-embed-text',
  chunk_size: 512,
  chunk_overlap: 64,
  concurrency: 4,
  query_prefix: 'search_query: ',
  document_prefix: null,
  base_url: null,
  is_default: false,
  has_api_key: false,
  configuration_keys: [],
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  ...overrides,
})

export const embeddingConfig = (
  overrides: Partial<AdminTeamEmbeddingProvidersConfig> = {}
): AdminTeamEmbeddingProvidersConfig => ({
  providers: [embeddingProvider()],
  coverage: {
    has_active_provider: true,
    active_model: 'nomic-embed-text',
    items: [
      {
        entity_type: 'prompt',
        total: 10,
        embedded: 8,
        pending: 2,
        embedded_percent: 80,
      },
    ],
  },
  ...overrides,
})

export const emailConfig = (
  overrides: Partial<AdminTeamEmailProviderConfig> = {}
): AdminTeamEmailProviderConfig => ({
  configured: true,
  source: 'team',
  effective_from_address: 'team@example.com',
  provider_type: 'smtp',
  from_address: 'team@example.com',
  from_name: 'Team Mail',
  reply_to: 'reply@example.com',
  settings: { smtp: { host: 'smtp.example.com', port: '587' } },
  has_secret: true,
  last_success_at: '2026-09-20T10:00:00Z',
  last_error_at: null,
  status: 'healthy',
  ...overrides,
})

export const githubConfig = (
  overrides: Partial<AdminTeamGitHubConfig> = {}
): AdminTeamGitHubConfig => ({
  app_config: {
    id: 'gh-1',
    app_id: '123456',
    app_slug: 'team-app',
    client_id: 'Iv1.abc',
    has_private_key: true,
    has_client_secret: false,
    has_webhook_secret: true,
    webhook_configured: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
  installation: {
    installed: true,
    account_login: 'acme',
    installation_id: 987654,
    suspended: false,
    installed_at: '2026-09-02T00:00:00Z',
  },
  ...overrides,
})

export const artifactTypes = (): AdminTeamArtifactTypes => ({
  types: [
    {
      id: 'at-1',
      slug: 'report',
      name: 'Report',
      is_system: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 'at-2',
      slug: 'runbook',
      name: 'Runbook',
      is_system: false,
      created_at: '2026-09-01T00:00:00Z',
      updated_at: '2026-09-01T00:00:00Z',
    },
  ],
})

export const auditPage = (
  overrides: Partial<AdminTeamSettingsAuditListResponse> = {}
): AdminTeamSettingsAuditListResponse => ({
  entries: [
    {
      id: 'a1',
      surface: 'model_provider',
      actor_user_id: 'u1',
      actor_name: 'Alice',
      source_team_id: 't2',
      source_team_name: 'Platform',
      source_resource_id: 'mp-9',
      created_resource_id: 'mp-10',
      detail: { created_name: 'Copied OpenAI', has_api_key: true },
      created_at: '2026-09-10T00:00:00Z',
    },
    {
      id: 'a2',
      surface: 'custom_types',
      actor_user_id: null,
      actor_name: null,
      source_team_id: '9b8c7d6e-0000-4000-8000-000000000002',
      source_team_name: null,
      source_resource_id: null,
      created_resource_id: null,
      detail: { added_slugs: ['runbook', 'adr'] },
      created_at: '2026-09-09T00:00:00Z',
    },
  ],
  total_count: 45,
  page: 1,
  per_page: 20,
  total_pages: 3,
  ...overrides,
})
