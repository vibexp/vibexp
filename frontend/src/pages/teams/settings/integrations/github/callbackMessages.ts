import { ApiError } from '@/types/errors'

/**
 * Messages for the install-callback failure arms (#485).
 *
 * The callback is the one step where several genuinely different things can go
 * wrong, and they need genuinely different actions: one is "ask someone else",
 * one is "disconnect the other team", one is "finish configuring your App", and
 * one is "your App is missing its OAuth credentials". Collapsing them into
 * "Failed to complete GitHub installation" — which is what happened before —
 * leaves the admin with nothing to do but retry, which never helps.
 */
export function describeCallbackFailure(error: unknown): string {
  if (!(error instanceof ApiError)) {
    return 'Failed to complete the GitHub installation. Please try again.'
  }

  switch (error.code) {
    case 'installation_already_connected':
      return 'This GitHub organization is already connected to another team. Each GitHub org or account can only be connected to one team — disconnect it there first.'

    case 'installation_not_authorized':
      return 'Your GitHub account cannot administer this installation. Ask someone who can install the App on that organization to complete the connection.'

    case 'GITHUB_APP_NOT_CONFIGURED':
      return 'This team has no GitHub App configured. Register one under Settings → Integrations → GitHub, then install it.'

    case 'github_user_auth_not_configured':
      return 'This team’s GitHub App has no OAuth credentials, so the installation cannot be verified. Add the App’s Client ID and secret in Settings → Integrations → GitHub, and enable “Request user authorization (OAuth) during installation” on GitHub.'

    default:
      break
  }

  // Fall back on status when the code is one this build does not know: a 403
  // here is always an authority problem, whatever the server called it.
  if (error.status === 403) {
    return 'You are not allowed to connect this GitHub installation to this team. It needs permission to manage the team, and access to the installation on GitHub.'
  }

  return 'Failed to complete the GitHub installation. Please try again.'
}

/** What to tell someone who reached the callback with no authorization code. */
export const MISSING_CODE_MESSAGE =
  'GitHub did not return an authorization code, so this installation cannot be verified. Start the install again from Settings → Integrations → GitHub.'
