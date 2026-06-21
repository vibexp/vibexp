/**
 * User utility functions
 */

/**
 * Check if a user is a first-time user based on their account creation timestamp.
 * A user is considered first-time if their account was created within the last 5 minutes.
 *
 * This is used for:
 * - Determining Welcome vs Welcome Back greeting on homepage
 * - Tracking first-time sign-in events in Google Analytics for conversion tracking
 *
 * @param createdAt - The user's account creation timestamp (ISO 8601 format)
 * @returns boolean - true if user's account is less than 5 minutes old, false otherwise
 */
export function isFirstTimeUser(createdAt: string | null | undefined): boolean {
  if (!createdAt) return false

  try {
    const accountAge = Date.now() - new Date(createdAt).getTime()
    const fiveMinutes = 5 * 60 * 1000
    return accountAge < fiveMinutes
  } catch (error) {
    console.error('Error parsing user created_at timestamp:', error)
    return false
  }
}
