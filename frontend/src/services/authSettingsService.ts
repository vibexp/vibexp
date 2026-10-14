import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type { AdminInstanceSettingsAuditPage } from './adminService'

// Instance authentication settings (#1238, page #1239): sign-in providers, the
// access allowlist, instance admin grants and their audit log. Every call is
// authorized server-side. A setup session (the `/setup` page) reaches the
// provider and allowlist calls only; the admin and audit calls are a 404 there.
export type AdminAuthProvider = components['schemas']['AdminAuthProvider']
export type AdminAuthProviderType =
  components['schemas']['AdminAuthProviderType']
export type AdminAuthProviderList =
  components['schemas']['AdminAuthProviderList']
export type AdminAuthProviderCreate =
  components['schemas']['AdminAuthProviderCreate']
export type AdminAuthProviderUpdate =
  components['schemas']['AdminAuthProviderUpdate']
export type AdminAuthProviderSaved =
  components['schemas']['AdminAuthProviderSaved']
export type AdminAuthProviderTestRequest =
  components['schemas']['AdminAuthProviderTestRequest']
export type AdminAuthProviderTestResult =
  components['schemas']['AdminAuthProviderTestResult']
export type AdminAuthAllowlist = components['schemas']['AdminAuthAllowlist']
export type AdminAuthAllowlistUpdate =
  components['schemas']['AdminAuthAllowlistUpdate']
export type AdminAuthAllowlistPreviewRequest =
  components['schemas']['AdminAuthAllowlistPreviewRequest']
export type AdminAuthAllowlistImpact =
  components['schemas']['AdminAuthAllowlistImpact']
export type AdminInstanceAdmin = components['schemas']['AdminInstanceAdmin']
export type AdminInstanceAdminList =
  components['schemas']['AdminInstanceAdminList']
export type AdminInstanceAdminGrant =
  components['schemas']['AdminInstanceAdminGrant']
export type AdminAuthSettingsAuditSetting =
  components['schemas']['AdminAuthSettingsAuditSetting']
export type AdminAuthPolicy = components['schemas']['AdminAuthPolicy']
export type AdminAuthPolicyUpdate =
  components['schemas']['AdminAuthPolicyUpdate']
export type AdminAuthRegistrationMode =
  components['schemas']['AdminAuthRegistrationMode']
export type AdminBootstrapPasswordRequest =
  components['schemas']['AdminBootstrapPasswordRequest']
export type AdminAuthProviderDeleteParams = NonNullable<
  operations['deleteAdminAuthProvider']['parameters']['query']
>
export type AdminAuthSettingsAuditParams = Omit<
  operations['listAdminAuthSettingsAudit']['parameters']['query'],
  'setting'
>

const PROVIDERS_PATH = '/api/v1/admin/settings/auth/providers'
const ALLOWLIST_PATH = '/api/v1/admin/settings/auth/allowlist'
const ADMINS_PATH = '/api/v1/admin/settings/auth/admins'
const POLICY_PATH = '/api/v1/admin/settings/auth/policy'
const BOOTSTRAP_PATH = '/api/v1/admin/settings/auth/bootstrap-admin'

class AuthSettingsService {
  /** Every stored sign-in provider, with the shared settings `version`. */
  async listProviders(): Promise<AdminAuthProviderList> {
    return unwrap(generatedClient.GET(PROVIDERS_PATH))
  }

  /** Store a new provider; 409 on a taken slug or a second google/github. */
  async createProvider(
    body: AdminAuthProviderCreate
  ): Promise<AdminAuthProviderSaved> {
    return unwrap(generatedClient.POST(PROVIDERS_PATH, { body }))
  }

  /**
   * Replace a provider's editable fields. 409 `lockout_risk` when the change
   * could lock sign-in out (re-send with `confirm_lockout_risk`), 409
   * `INSTANCE_SETTINGS_VERSION_CONFLICT` on a stale `expected_version`.
   */
  async updateProvider(
    id: string,
    body: AdminAuthProviderUpdate
  ): Promise<AdminAuthProviderSaved> {
    return unwrap(
      generatedClient.PUT(`${PROVIDERS_PATH}/{id}`, {
        params: { path: { id } },
        body,
      })
    )
  }

  /** Remove a provider; the same two 409s as an update. */
  async deleteProvider(
    id: string,
    query: AdminAuthProviderDeleteParams
  ): Promise<void> {
    await unwrap(
      generatedClient.DELETE(`${PROVIDERS_PATH}/{id}`, {
        params: { path: { id }, query },
      })
    )
  }

  /**
   * Check a stored provider (`id`), a stored provider with overrides, or an
   * unsaved candidate. Nothing is stored; a failed check is `is_valid: false`.
   */
  async testProvider(
    body: AdminAuthProviderTestRequest
  ): Promise<AdminAuthProviderTestResult> {
    return unwrap(generatedClient.POST(`${PROVIDERS_PATH}/test`, { body }))
  }

  /** The stored access allowlist; both lists empty means open access. */
  async getAllowlist(): Promise<AdminAuthAllowlist> {
    return unwrap(generatedClient.GET(ALLOWLIST_PATH))
  }

  /** Replace the allowlist; 409 on a stale `expected_version`. */
  async updateAllowlist(
    body: AdminAuthAllowlistUpdate
  ): Promise<AdminAuthAllowlist> {
    return unwrap(generatedClient.PUT(ALLOWLIST_PATH, { body }))
  }

  /** Remove the stored allowlist, reverting to open access. */
  async resetAllowlist(): Promise<void> {
    await unwrap(generatedClient.DELETE(ALLOWLIST_PATH))
  }

  /** Who a candidate allowlist would sign out. Nothing is stored. */
  async previewAllowlist(
    body: AdminAuthAllowlistPreviewRequest
  ): Promise<AdminAuthAllowlistImpact> {
    return unwrap(generatedClient.POST(`${ALLOWLIST_PATH}/preview`, { body }))
  }

  /**
   * The authentication policy: whether email and password sign-in is on and
   * who may get an account. Never a 404: the defaults come back with a null
   * `version` when nothing is stored. Instance admins only.
   */
  async getPolicy(): Promise<AdminAuthPolicy> {
    return unwrap(generatedClient.GET(POLICY_PATH))
  }

  /**
   * Replace the policy. 409 `INSTANCE_SETTINGS_VERSION_CONFLICT` on a stale
   * `expected_version`; 409 `lockout_risk` when switching password sign-in off
   * could lock people out (re-send with `confirm_lockout_risk`).
   */
  async updatePolicy(body: AdminAuthPolicyUpdate): Promise<AdminAuthPolicy> {
    return unwrap(generatedClient.PUT(POLICY_PATH, { body }))
  }

  /**
   * Setup session only: give a root instance admin a password and switch
   * email and password sign-in on. 400 with a field error on `email` when the
   * address is not a root admin, or on `password` when it breaks the policy.
   */
  async bootstrapAdminPassword(
    body: AdminBootstrapPasswordRequest
  ): Promise<void> {
    await unwrap(generatedClient.PUT(BOOTSTRAP_PATH, { body }))
  }

  /** The root admins' emails and the DB-granted admins. */
  async listAdmins(): Promise<AdminInstanceAdminList> {
    return unwrap(generatedClient.GET(ADMINS_PATH))
  }

  /** Make an existing user an instance admin. Root admins only (403). */
  async grantAdmin(body: AdminInstanceAdminGrant): Promise<AdminInstanceAdmin> {
    return unwrap(generatedClient.POST(ADMINS_PATH, { body }))
  }

  /** Remove a DB grant. Root admins only (403). */
  async revokeAdmin(userId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE(`${ADMINS_PATH}/{user_id}`, {
        params: { path: { user_id: userId } },
      })
    )
  }

  /** One page of one authentication setting's audit log, newest first. */
  async listAudit(
    setting: AdminAuthSettingsAuditSetting,
    params: AdminAuthSettingsAuditParams = {}
  ): Promise<AdminInstanceSettingsAuditPage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/settings/auth/audit', {
        params: { query: { ...params, setting } },
      })
    )
  }
}

export const authSettingsService = new AuthSettingsService()
