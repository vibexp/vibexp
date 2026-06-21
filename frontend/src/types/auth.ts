// Authentication types
export interface User {
  id: string
  google_id?: string
  email: string
  name: string
  avatar_url?: string
  created_at: string
  updated_at: string
  onboarding_completed: boolean
  onboarding_completed_at?: string
}

export interface AuthState {
  user: User | null
  isAuthenticated: boolean
}

// WorkOS login URL response
export interface LoginUrlResponse {
  url: string
}

// Logout response
export interface LogoutResponse {
  message: string
}

// Legacy Google OAuth types (kept for backward compatibility during migration)
/** @deprecated Use LoginUrlResponse instead */
export interface GoogleLoginResponse {
  login_url: string
  state: string
}

/** @deprecated No longer returned by backend — backend sets httpOnly cookie */
export interface GoogleCallbackRequest {
  code: string
  state: string
}

/** @deprecated Backend no longer returns a token — session cookie is set server-side */
export interface AuthResponse {
  user: User
}

// API Key types
export interface APIKey {
  id: string
  user_id: string
  name: string
  key_prefix: string
  integrations: string[] // Array of integration codes
  is_legacy: boolean
  migration_notes?: string
  last_used_at?: string | null
  created_at: string
  updated_at: string
  // Legacy field for backward compatibility
  usage_type?: 'ai_tools' | 'cli' | 'mcp' | 'everything'
}

export interface CreateAPIKeyRequest {
  name: string
  integration_codes: string[] // Array of integration codes to grant access
}

export interface CreateAPIKeyResponse {
  api_key: APIKey
  full_key: string
  key_prefix: string
}

// Integration definition for UI
export interface Integration {
  code: string
  name: string
  description: string
}
