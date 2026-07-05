/**
 * Storage Key Constants
 *
 * Centralized management of all localStorage and sessionStorage keys.
 * This prevents typos, makes refactoring easier, and improves type safety.
 *
 * ## How to Add a New Key
 * 1. Add a new constant to the appropriate section below
 * 2. Use a descriptive, namespaced name (e.g., 'vx_feature_name')
 * 3. Document what data is stored and why
 * 4. Add the key to the STORAGE_KEYS object
 *
 * ## How to Change a Key
 * 1. Update the constant value in STORAGE_KEYS
 * 2. Consider migration logic if changing an existing key
 * 3. Update any related documentation
 *
 * ## How to Remove a Key
 * 1. Remove the constant from STORAGE_KEYS
 * 2. Add cleanup logic to migrate/remove old data if needed
 * 3. Update this comment
 *
 * ## Key Naming Convention
 * - Use 'vx_' prefix for VibeXP-specific keys (prevents collisions)
 * - Use snake_case for key names
 * - Be descriptive but concise
 *
 * ## Storage Type Guidelines
 * - localStorage: Use for persistent data across sessions (auth tokens, preferences)
 * - sessionStorage: Use for temporary, session-scoped data (redirect states, UI states)
 */

/**
 * All storage keys used in the application
 *
 * IMPORTANT: All keys use the 'vx_' prefix to prevent collisions with other
 * applications that might use localStorage on the same domain.
 */
export const STORAGE_KEYS = {
  // Team Management
  /** Currently selected team ID - persists across sessions */
  CURRENT_TEAM_ID: 'vx_current_team_id',
  /**
   * Globally selected project ID for the header project selector - persists
   * across sessions. Absent when "All projects" is selected. Cleared whenever
   * the current team changes (a project always belongs to one team).
   */
  CURRENT_PROJECT_ID: 'vx_current_project_id',
  /** Pending team invitation token - used during auth flow */
  PENDING_INVITATION_TOKEN: 'vx_pending_invitation_token',
  /**
   * Set of invitation IDs the user has dismissed from the dashboard banner.
   * Stored as a JSON-encoded `string[]` in sessionStorage so it resets on
   * tab close. Reading: `sessionStore.getJSON<string[]>(...)`.
   */
  INVITATION_BANNER_DISMISSED: 'vx_invitation_banner_dismissed',
  /**
   * Stash of `{ team_id, team_name }` set by the AcceptInvitation page after
   * a successful accept. Read once by the in-app handshake component
   * (inside TeamProvider) which switches the active team and shows the
   * welcome toast, then removes the key.
   */
  INVITATION_JUST_ACCEPTED: 'vx_invitation_just_accepted',

  // Analytics
  /** Referrer URL for page tracking - session scoped */
  ANALYTICS_REFERRER: 'vx_analytics_referrer',
  /** Flag to track if purchase event was sent - session scoped */
  PURCHASE_TRACKED: 'vx_purchase_tracked',
  /** Provider used in the current sign-in flow (e.g. 'Google', 'GitHub') - session scoped */
  LOGIN_METHOD: 'vx_login_method',

  // Login redirect
  /**
   * Same-origin path to return to after a login round-trip (e.g. the OAuth
   * consent page that bounced a signed-out visitor to sign-in). Stashed before
   * the provider redirect so it survives the IdP round-trip; read-and-cleared
   * once the user lands back authenticated. Session scoped.
   */
  RETURN_TO: 'vx_return_to',

  // Cookie Consent
  /** User's cookie consent decision (granted/denied) with timestamp */
  COOKIE_CONSENT: 'vx_cookie_consent',

  // Push Notifications
  /** FCM registration token for web push notifications */
  FCM_TOKEN: 'vx_fcm_token',
} as const

/**
 * Legacy key mappings for migration
 * These are the old keys that may still exist in user browsers.
 * Migration logic should check these and migrate to new keys.
 */
export const LEGACY_STORAGE_KEYS = {
  AUTH_TOKEN: 'auth_token',
  CURRENT_TEAM_ID: 'vibexp_current_team_id',
  PENDING_INVITATION_TOKEN: 'pending_invitation_token',
  INVITATION_BANNER_DISMISSED: 'invitation_banner_dismissed',
  ANALYTICS_REFERRER: 'analytics_referrer',
  PURCHASE_TRACKED: 'purchase_tracked',
  COOKIE_CONSENT: 'cookieConsent',
} as const

/**
 * Type representing all valid storage keys
 */
export type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS]

/**
 * Storage type categorization
 */
export const LOCAL_STORAGE_KEYS: ReadonlySet<StorageKey> = new Set([
  STORAGE_KEYS.CURRENT_TEAM_ID,
  STORAGE_KEYS.CURRENT_PROJECT_ID,
  STORAGE_KEYS.COOKIE_CONSENT,
  STORAGE_KEYS.FCM_TOKEN,
])

export const SESSION_STORAGE_KEYS: ReadonlySet<StorageKey> = new Set([
  STORAGE_KEYS.PENDING_INVITATION_TOKEN,
  STORAGE_KEYS.INVITATION_BANNER_DISMISSED,
  STORAGE_KEYS.INVITATION_JUST_ACCEPTED,
  STORAGE_KEYS.ANALYTICS_REFERRER,
  STORAGE_KEYS.PURCHASE_TRACKED,
  STORAGE_KEYS.LOGIN_METHOD,
  STORAGE_KEYS.RETURN_TO,
])
