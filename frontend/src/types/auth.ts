import type { User } from '../services/authService'

// UI-only auth state shape (not a wire type). The `User` wire type and the
// APIKey/provider wire types now live on their services (authService,
// apiKeyService) as generated re-exports.
export interface AuthState {
  user: User | null
  isAuthenticated: boolean
}

// Integration definition for UI
export interface Integration {
  code: string
  name: string
  description: string
}
