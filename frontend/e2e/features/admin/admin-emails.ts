/**
 * Instance-admin e2e identities (#317).
 *
 * ADMIN_EMAIL MUST match `INSTANCE_ADMIN_EMAILS` on the backend service in
 * `docker-compose.e2e.yml` — dev-logging in with it yields an instance admin.
 * Any other address (NON_ADMIN_EMAIL) is a regular user.
 */
export const ADMIN_EMAIL = 'admin-e2e@vibexp.test'
export const NON_ADMIN_EMAIL = 'nonadmin-e2e@vibexp.test'
