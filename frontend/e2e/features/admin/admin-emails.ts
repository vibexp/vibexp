/**
 * Instance-admin e2e identities (#317).
 *
 * ADMIN_EMAIL MUST match `INSTANCE_ADMIN_EMAILS` on the backend service in
 * `docker-compose.e2e.yml` — dev-logging in with it yields an instance admin.
 * Any other address (NON_ADMIN_EMAIL) is a regular user.
 */
export const ADMIN_EMAIL = 'admin-e2e@vibexp.test'
/**
 * The display name every spec dev-logs ADMIN_EMAIL in with. Dev login only
 * sets the name when it CREATES the user, so whichever spec runs first fixes
 * it for the whole run — a spec asserting the admin's name (the audit actor in
 * the Settings → Email journey, #1191) must use this, and so must every other
 * login as ADMIN_EMAIL, or the assertion depends on spec order.
 */
export const ADMIN_NAME = 'Admin E2E'
export const NON_ADMIN_EMAIL = 'nonadmin-e2e@vibexp.test'
