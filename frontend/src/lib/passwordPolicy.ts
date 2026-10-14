/**
 * The instance's password rule as the pages state it (epic #1426). The server
 * is the authority (`internal/auth/password`): these only word the rule under
 * a "new password" field and catch a slip before a request. Shared by every
 * page that takes a new password, so they cannot drift apart.
 */

/** The written rule, for the hint under a new-password field. */
export const PASSWORD_RULE =
  'At least 12 characters. Not a common password, and not your email address.'

export const PASSWORD_MIN_LENGTH = 12

/** The API's bound on a password field, in characters. */
export const PASSWORD_MAX_LENGTH = 1024
