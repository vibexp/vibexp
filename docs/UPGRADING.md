# Upgrading VibeXP

Breaking changes and the migration each one needs, newest first. Anything listed
here requires action **before** the new image will start; everything else
upgrades in place.

## Unreleased — GitHub App configuration moved from `config.yaml` to per-team settings

**You must delete the `github:` section from your `config.yaml`, or the backend
will refuse to start.**

GitHub App credentials used to be instance-wide: one App in `config.yaml`, shared
by every team. They are now registered **per team** and stored encrypted in the
database, so each team connects its own GitHub App to its own organization.

### What to do

1. **Delete the whole top-level `github:` block** from your `config.yaml`:

   ```yaml
   # DELETE THIS ENTIRE SECTION
   github:
     app_id: "..."
     app_slug: "..."
     app_private_key: "..."
     webhook_url: "..."
     webhook_secret: "..."
     app_client_id: "..."
     app_client_secret: "..."
   ```

   Leaving it in place is a **hard startup failure** with an error naming the
   section — deliberately loud, so nobody runs on credentials that are no longer
   read.

   > ⚠️ Do **not** touch `auth.github`. That is the GitHub **web-login** OAuth
   > client — a different credential set on a different code path — and it is
   > unaffected by this change.

2. **Drop the now-unused environment variables** (harmless if left set, but they
   do nothing): `GITHUB_APP_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_PRIVATE_KEY`,
   `GITHUB_WEBHOOK_URL`, `GITHUB_WEBHOOK_SECRET`, `GITHUB_APP_CLIENT_ID`,
   `GITHUB_APP_CLIENT_SECRET`.

   Keep `GITHUB_CLIENT_SECRET` — that one belongs to `auth.github`, the web login.

3. **Re-register the App on each team** that wants the GitHub integration:
   **Settings → Integrations → GitHub**. You can reuse the App you already have —
   paste the same App ID, slug, client ID, client secret, and private key (raw
   PEM or base64, both accepted). VibeXP generates a fresh webhook secret and a
   per-App webhook URL for you to paste into the App's settings on GitHub.

   Note that a GitHub App has exactly one webhook URL, so **one App can be
   registered by one team only**. Teams that need separate integrations need
   separate GitHub Apps.

4. **Re-connect the installation** on each team after registering the App.

### Blast radius

Small in practice: the UI connect flow had been failing since the caller-authority
hardening (the SPA never relayed GitHub's `code`), so most instances have no
working installation to lose. Existing installation rows were already cleared by
the accompanying migration.
