import type {
  CreateGitHubAppConfigRequest,
  GitHubAppValidationErrorCode,
} from '@/services/githubAppConfigService'

/** Form field a validation failure should be attributed to, when there is one. */
export type GitHubAppField = keyof CreateGitHubAppConfigRequest

export interface ValidationFailure {
  title: string
  description: string
  /**
   * The field to attach the error to. Undefined means the failure is not about
   * any single field (e.g. the network), so it belongs in the dialog banner
   * rather than under an input.
   */
  field?: GitHubAppField
}

/**
 * Maps the backend's fixed validation categories to something a person can act
 * on.
 *
 * The categories are deliberately coarse (#464: the probe must not become an
 * oracle for what the server can reach), which means the server's own `message`
 * is generic by design. Turning each category into a specific instruction — and
 * pointing it at the field that is actually wrong — is this layer's job; a
 * generic "validation failed" toast would waste the whole mechanism.
 */
const FAILURES: Record<GitHubAppValidationErrorCode, ValidationFailure> = {
  invalid_credentials: {
    title: 'GitHub rejected these credentials',
    description:
      'The private key does not belong to this App ID, or the key is unusable. Re-copy the App ID from the App’s General page, then generate a fresh private key and paste the new .pem.',
    field: 'private_key',
  },
  app_not_found: {
    title: 'No GitHub App with that ID',
    description:
      'GitHub has no App with this ID, or the private key authenticates a different one. Check the App ID against the App’s General page.',
    field: 'app_id',
  },
  slug_mismatch: {
    title: 'The App slug does not match',
    description:
      'The credentials are valid, but they belong to an App with a different slug. Copy the slug from the App’s public URL (github.com/apps/<slug>) — not its display name.',
    field: 'app_slug',
  },
  insufficient_permissions: {
    title: 'The App is missing a required permission',
    description:
      'Grant Contents (read-only) and Metadata (read-only) on the App’s Permissions page, then verify again. Existing installations must accept the new permission on GitHub before it takes effect.',
  },
  connection_failed: {
    title: 'Could not reach GitHub',
    description:
      'VibeXP could not connect to the GitHub API. Check outbound network access and any egress proxy from this instance, then try again.',
  },
}

/**
 * Returns an actionable description of a failed validate probe.
 *
 * `serverMessage` is the fallback for a category this build does not know
 * about: the enum can gain a member server-side before the SPA is redeployed,
 * and showing the server's own wording beats showing nothing.
 */
export function describeValidationFailure(
  code: GitHubAppValidationErrorCode | undefined,
  serverMessage?: string
): ValidationFailure {
  if (code && code in FAILURES) {
    return FAILURES[code]
  }
  return {
    title: 'The GitHub App could not be validated',
    description:
      serverMessage ??
      'GitHub did not accept these credentials. Check the App ID, slug, and private key.',
  }
}
