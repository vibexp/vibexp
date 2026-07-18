/**
 * Google Tag Manager Analytics Type Definitions
 *
 * This file contains all TypeScript interfaces and types for GTM event tracking
 * in the VibeXP frontend application. It provides type safety for analytics events
 * and ensures consistent event structure across the application.
 */

// User identification and properties
export interface UserProperties {
  user_id: string
  email: string
  name: string
  signup_date?: string | null
  avatar_url?: string | null
  created_at?: string | null
}

// Base event structure that all events should extend
export interface BaseEvent {
  event: string
  timestamp: number
  page_path: string
  page_title?: string
  user_properties?: UserProperties
  session_id?: string
  environment: 'development' | 'production'
}

// Authentication related events
export interface AuthenticationEvent extends BaseEvent {
  event:
    | 'user_signin_page_view'
    | 'user_signed_in'
    | 'user_signed_in_first_time'
    | 'user_logged_out'
  user_properties?: UserProperties
}

// Page view tracking
export interface PageViewEvent extends BaseEvent {
  event: 'page_view'
  page_path: string
  page_title: string
  referrer?: string
  user_properties?: UserProperties
}

// User interaction events (for future expansion)
export interface UserInteractionEvent extends BaseEvent {
  event: 'user_interaction'
  interaction_type: string
  element_id?: string
  element_class?: string
  element_text?: string
  user_properties?: UserProperties
}

// Error tracking events
export interface ErrorEvent extends BaseEvent {
  event: 'javascript_error' | 'analytics_error'
  error_message: string
  error_stack?: string
  error_component?: string
  user_properties?: UserProperties
}

// Feature-specific events for DL-190

// Prompts Feature Events
export interface PromptEvent extends BaseEvent {
  event:
    | 'prompts_page_view'
    | 'prompt_created'
    | 'prompt_updated'
    | 'reusable_prompt_modal_triggered'
    | 'prompt_preview_viewed'
    | 'prompt_content_copied'
    | 'shared_prompt_viewed'
    | 'shared_prompt_copy_clicked'
    | 'shared_prompt_cta_clicked'
    | 'shared_prompt_error'
  prompt_id?: string
  prompt_title?: string
  prompt_type?: string
  share_token?: string
  share_type?: 'public' | 'restricted'
  error_message?: string
  has_expiration?: boolean
  referrer?: string
  action_context:
    | 'create'
    | 'update'
    | 'view'
    | 'copy'
    | 'preview'
    | 'modal_trigger'
    | 'cta_click'
    | 'error'
  user_properties?: UserProperties
}

// Shared CRUD action context used by resource feature events
export type CrudActionContext = 'create' | 'update' | 'view' | 'delete'

// Artifacts Feature Events
export interface ArtifactEvent extends BaseEvent {
  event:
    | 'artifacts_page_view'
    | 'artifact_created'
    | 'artifact_viewed'
    | 'artifact_updated'
  artifact_id?: string
  artifact_type?: string
  artifact_title?: string
  action_context: CrudActionContext
  user_properties?: UserProperties
}

// Memory Feature Events
export interface MemoryEvent extends BaseEvent {
  event:
    | 'memories_page_view'
    | 'memory_created'
    | 'memory_updated'
    | 'memory_viewed'
  memory_id?: string
  memory_type?: string
  action_context: CrudActionContext
  user_properties?: UserProperties
}

// Blueprint Feature Events
export interface BlueprintEvent extends BaseEvent {
  event:
    | 'blueprint_page_view'
    | 'blueprint_created'
    | 'blueprint_viewed'
    | 'blueprint_updated'
  blueprint_id?: string
  blueprint_type?: string
  blueprint_title?: string
  action_context: CrudActionContext
  user_properties?: UserProperties
}

// Claude Code Feature Events
export interface ClaudeCodeEvent extends BaseEvent {
  event:
    | 'claude_code_setup'
    | 'claude_code_sessions_view'
    | 'claude_code_session_clicked'
  session_id?: string
  setup_step?: string
  action_context: 'setup' | 'sessions_view' | 'session_click'
  user_properties?: UserProperties
}

// Cursor IDE Feature Events
export interface CursorIDEEvent extends BaseEvent {
  event:
    | 'cursor_ide_setup'
    | 'cursor_ide_sessions_view'
    | 'cursor_ide_session_clicked'
  session_id?: string
  setup_step?: string
  action_context: 'setup' | 'sessions_view' | 'session_click'
  user_properties?: UserProperties
}

// API Management Events
export interface APIKeyEvent extends BaseEvent {
  event: 'api_key_created' | 'api_key_deleted'
  api_key_id?: string
  action_context: 'create' | 'delete'
  user_properties?: UserProperties
}

// MCP Integration Events
export interface MCPEvent extends BaseEvent {
  event: 'mcp_server_page_view' | 'mcp_config_copied'
  config_type?: string
  action_context: 'page_view' | 'config_copy'
  user_properties?: UserProperties
}
// Union type for all possible events
export type AnalyticsEvent =
  | AuthenticationEvent
  | PageViewEvent
  | UserInteractionEvent
  | ErrorEvent
  | PromptEvent
  | ArtifactEvent
  | MemoryEvent
  | BlueprintEvent
  | ClaudeCodeEvent
  | CursorIDEEvent
  | APIKeyEvent
  | MCPEvent

// GTM data layer interface
export interface GTMDataLayer extends Record<string, unknown> {
  event?: string
  timestamp?: number
  page_path?: string
  page_title?: string
  user_properties?: UserProperties
  session_id?: string
  environment?: 'development' | 'production'
}

// Analytics service configuration
export interface AnalyticsConfig {
  gtmId: string
  enabled: boolean
  debug: boolean
  environment: 'development' | 'production'
  enableConsoleLogging: boolean
  enableErrorTracking: boolean
}

// Event tracking method parameters
export interface TrackEventParams {
  event: string
  properties?: Record<string, unknown>
  userProperties?: UserProperties
}

// Page tracking parameters
export interface TrackPageParams {
  path: string
  title: string
  referrer?: string
  userProperties?: UserProperties
}

// Authentication tracking parameters
export interface TrackAuthParams {
  eventType:
    | 'signin_page_view'
    | 'signed_in'
    | 'signed_in_first_time'
    | 'logged_out'
  userProperties?: UserProperties
}

// Error tracking parameters
export interface TrackErrorParams {
  error: Error
  component?: string
  additionalInfo?: Record<string, unknown>
}

// Analytics service interface
export interface AnalyticsService {
  // Core tracking methods
  track(event: AnalyticsEvent): void
  trackEvent(params: TrackEventParams): void
  trackPage(params: TrackPageParams): void
  trackAuth(params: TrackAuthParams): void
  trackError(params: TrackErrorParams): void

  // User management
  identify(userProperties: UserProperties): void
  setUserProperties(properties: Partial<UserProperties>): void
  clearUser(): void

  // Configuration
  configure(config: Partial<AnalyticsConfig>): void
  isEnabled(): boolean
  getConfig(): AnalyticsConfig

  // Utility methods
  generateSessionId(): string
  getCurrentPagePath(): string
  getCurrentPageTitle(): string
}

// React hooks return types
export interface UseAnalyticsReturn {
  track: (event: AnalyticsEvent) => void
  trackEvent: (params: TrackEventParams) => void
  trackPage: (params: TrackPageParams) => void
  trackAuth: (params: TrackAuthParams) => void
  trackError: (params: TrackErrorParams) => void
  identify: (userProperties: UserProperties) => void
  isEnabled: boolean
}

export interface UsePageTrackingReturn {
  trackCurrentPage: () => void
  trackPageView: (path?: string, title?: string) => void
}

// Constants for event names - ensures consistency
export const ANALYTICS_EVENTS = {
  // Authentication events
  USER_SIGNIN_PAGE_VIEW: 'user_signin_page_view',
  USER_SIGNED_IN: 'user_signed_in',
  USER_SIGNED_IN_FIRST_TIME: 'user_signed_in_first_time',
  USER_LOGGED_OUT: 'user_logged_out',

  // Page events
  PAGE_VIEW: 'page_view',

  // Error events
  JAVASCRIPT_ERROR: 'javascript_error',
  ANALYTICS_ERROR: 'analytics_error',

  // User interaction events (for future use)
  USER_INTERACTION: 'user_interaction',

  // Feature-specific events for DL-190

  // Prompts feature events
  PROMPTS_PAGE_VIEW: 'prompts_page_view',
  PROMPT_CREATED: 'prompt_created',
  PROMPT_UPDATED: 'prompt_updated',
  REUSABLE_PROMPT_MODAL_TRIGGERED: 'reusable_prompt_modal_triggered',
  PROMPT_PREVIEW_VIEWED: 'prompt_preview_viewed',
  PROMPT_CONTENT_COPIED: 'prompt_content_copied',
  SHARED_PROMPT_VIEWED: 'shared_prompt_viewed',
  SHARED_PROMPT_COPY_CLICKED: 'shared_prompt_copy_clicked',
  SHARED_PROMPT_CTA_CLICKED: 'shared_prompt_cta_clicked',
  SHARED_PROMPT_ERROR: 'shared_prompt_error',

  // Artifacts feature events
  ARTIFACTS_PAGE_VIEW: 'artifacts_page_view',
  ARTIFACT_CREATED: 'artifact_created',
  ARTIFACT_VIEWED: 'artifact_viewed',
  ARTIFACT_UPDATED: 'artifact_updated',

  // Memory feature events
  MEMORIES_PAGE_VIEW: 'memories_page_view',
  MEMORY_CREATED: 'memory_created',
  MEMORY_UPDATED: 'memory_updated',
  MEMORY_VIEWED: 'memory_viewed',

  // Blueprint feature events
  BLUEPRINT_PAGE_VIEW: 'blueprint_page_view',
  BLUEPRINT_CREATED: 'blueprint_created',
  BLUEPRINT_VIEWED: 'blueprint_viewed',
  BLUEPRINT_UPDATED: 'blueprint_updated',
  BLUEPRINT_DELETED: 'blueprint_deleted',

  // Claude Code feature events
  CLAUDE_CODE_SETUP_PAGE_VIEW: 'claude_code_setup_page_view',
  CLAUDE_CODE_SETUP: 'claude_code_setup',
  CLAUDE_CODE_SESSIONS_PAGE_VIEW: 'claude_code_sessions_page_view',
  CLAUDE_CODE_SESSION_VIEWED: 'claude_code_session_viewed',
  CLAUDE_CODE_SESSION_DETAIL_VIEW: 'claude_code_session_detail_view',

  // Cursor IDE feature events
  CURSOR_IDE_SETUP_PAGE_VIEW: 'cursor_ide_setup_page_view',
  CURSOR_IDE_SETUP: 'cursor_ide_setup',
  CURSOR_IDE_SESSIONS_PAGE_VIEW: 'cursor_ide_sessions_page_view',
  CURSOR_IDE_SESSION_VIEWED: 'cursor_ide_session_viewed',
  CURSOR_IDE_SESSION_DETAIL_VIEW: 'cursor_ide_session_detail_view',

  // API Management events
  API_KEYS_PAGE_VIEW: 'api_keys_page_view',
  API_KEY_CREATED: 'api_key_created',
  API_KEY_DELETED: 'api_key_deleted',
  API_KEY_COPIED: 'api_key_copied',

  // MCP Integration events
  MCP_SERVERS_PAGE_VIEW: 'mcp_servers_page_view',
  MCP_SERVER_ADD: 'mcp_server_add',
  MCP_SERVER_CONFIGURED: 'mcp_server_configured',
  MCP_SERVER_CONNECTION_TESTED: 'mcp_server_connection_tested',
  MCP_INTEGRATION_PAGE_VIEW: 'mcp_integration_page_view',
  MCP_CONFIG_SECTION_EXPANDED: 'mcp_config_section_expanded',
  MCP_TOOL_EXPANDED: 'mcp_tool_expanded',
  MCP_CONFIG_COPIED: 'mcp_config_copied',

  // GitHub Integration events
  GITHUB_INTEGRATION_PAGE_VIEW: 'github_integration_page_view',
  GITHUB_CONNECTED: 'github_connected',
  GITHUB_DISCONNECTED: 'github_disconnected',
  GITHUB_REPOSITORIES_VIEWED: 'github_repositories_viewed',

  // Feed feature events
  FEED_PAGE_VIEW: 'feed_page_view',
  FEED_ITEM_VIEWED: 'feed_item_viewed',
  FEED_ITEM_ARCHIVED: 'feed_item_archived',
  FEED_ITEM_UNARCHIVED: 'feed_item_unarchived',
  FEED_ITEM_DELETED: 'feed_item_deleted',
  FEED_ITEM_POSTED: 'feed_item_posted',
  FEED_CREATED: 'feed_created',
  FEED_UPDATED: 'feed_updated',
  FEED_DELETED: 'feed_deleted',
} as const

export type AnalyticsEventType =
  (typeof ANALYTICS_EVENTS)[keyof typeof ANALYTICS_EVENTS]

// Environment detection
export const ANALYTICS_ENVIRONMENT = {
  DEVELOPMENT: 'development',
  PRODUCTION: 'production',
} as const

export type AnalyticsEnvironment =
  (typeof ANALYTICS_ENVIRONMENT)[keyof typeof ANALYTICS_ENVIRONMENT]
