import { ApiError } from '../types/errors'
import { readStringMeta } from './apiErrorMeta'

/**
 * User-facing message pair for an invitation error state.
 *
 * Shared by the AcceptInvitation page, the dashboard banner, and the
 * `useAcceptAndEnterTeam` hook so they all surface the same wording.
 */
export interface InvitationErrorView {
  title: string
  description: string
}

export const GENERIC_LOAD_ERROR: InvitationErrorView = {
  title: "Couldn't load invitation",
  description: "Couldn't load invitation. Please try again later.",
}

export const SESSION_EXPIRED_ERROR: InvitationErrorView = {
  title: 'Sign in required',
  description: 'Your session has expired. Please sign in again to continue.',
}

export const GENERIC_ACCEPT_ERROR: InvitationErrorView = {
  title: "Couldn't accept invitation",
  description: 'Failed to accept invitation. Please try again.',
}

export const INVALID_LINK_ERROR: InvitationErrorView = {
  title: 'Invalid invitation',
  description: 'Invalid invitation link',
}

const NOT_FOUND_ERROR: InvitationErrorView = {
  title: 'Invitation not found',
  description: 'Invitation not found.',
}

const EXPIRED_ERROR: InvitationErrorView = {
  title: 'Invitation expired',
  description: 'This invitation has expired.',
}

const REVOKED_ERROR: InvitationErrorView = {
  title: 'Invitation revoked',
  description: 'This invitation has been revoked.',
}

const ALREADY_ACCEPTED_ERROR: InvitationErrorView = {
  title: 'Invitation no longer available',
  description: 'This invitation has already been accepted.',
}

const ALREADY_REJECTED_ERROR: InvitationErrorView = {
  title: 'Invitation no longer available',
  description: 'This invitation has already been rejected.',
}

const NO_LONGER_VALID_ERROR: InvitationErrorView = {
  title: 'Invitation no longer available',
  description: 'This invitation is no longer valid.',
}

/**
 * Wrong-email mismatch when the signed-in user does not match the invitee.
 * Detail string includes the actual addresses when the backend supplies them
 * via metadata (see {@link mapInvitationError}); otherwise we fall back to a
 * generic explanation.
 */
function buildEmailMismatchError(
  invitee?: string,
  actor?: string
): InvitationErrorView {
  if (invitee && actor) {
    return {
      title: 'Wrong account',
      description: `This invitation is for ${invitee}. You're signed in as ${actor}. Sign out and back in with ${invitee} to accept.`,
    }
  }
  return {
    title: 'Wrong account',
    description:
      "This invitation is for a different email address than the one you're signed in with.",
  }
}

function looksLikeEmailMismatch(error: ApiError): boolean {
  const code = error.code.toUpperCase()
  if (
    code === 'INVITATION_EMAIL_MISMATCH' ||
    code === 'INVITEE_EMAIL_MISMATCH'
  ) {
    return true
  }
  const detail = error.response.detail.toLowerCase()
  // Heuristic: the backend may not yet return a distinct code, so match on
  // common phrases. Tracked as a deferred finding for the backend to provide
  // a stable code/metadata pair.
  return (
    detail.includes('email') &&
    (detail.includes('mismatch') ||
      detail.includes('does not match') ||
      detail.includes("doesn't match"))
  )
}

/**
 * Map an error thrown by an invitation API call to a user-facing alert.
 *
 * Mapping is status-driven (more robust than title/code strings):
 *   401 → session expired (cookie evicted between mount and the GET)
 *   404 → not found
 *   410 → expired pending invitation
 *   409 → revoked / already accepted / already rejected (disambiguated via
 *         metadata.status, falling back to a generic "no longer valid"
 *         message). 409 + email-mismatch heuristic → wrong-account view.
 *   403 → also surfaced as wrong-account when the heuristic matches; otherwise
 *         falls through to the generic error.
 *   anything else (network, 5xx) → generic retry message
 */
export function mapInvitationError(err: unknown): InvitationErrorView {
  if (!(err instanceof ApiError)) {
    return GENERIC_LOAD_ERROR
  }

  if (err.status === 401) {
    return SESSION_EXPIRED_ERROR
  }

  if (err.status === 404) {
    return NOT_FOUND_ERROR
  }

  if (err.status === 410) {
    return EXPIRED_ERROR
  }

  if (err.status === 403 && looksLikeEmailMismatch(err)) {
    return buildEmailMismatchError(
      readStringMeta(err, 'invitee_email', 'expected_email'),
      readStringMeta(err, 'actor_email', 'current_email', 'authenticated_email')
    )
  }

  if (err.status === 409) {
    if (looksLikeEmailMismatch(err)) {
      return buildEmailMismatchError(
        readStringMeta(err, 'invitee_email', 'expected_email'),
        readStringMeta(
          err,
          'actor_email',
          'current_email',
          'authenticated_email'
        )
      )
    }

    const metadataStatus = readStringMeta(err, 'status')

    if (metadataStatus === 'revoked') {
      return REVOKED_ERROR
    }
    if (metadataStatus === 'accepted') {
      return ALREADY_ACCEPTED_ERROR
    }
    if (metadataStatus === 'rejected') {
      return ALREADY_REJECTED_ERROR
    }
    return NO_LONGER_VALID_ERROR
  }

  return GENERIC_LOAD_ERROR
}
