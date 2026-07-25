import { ExternalLink } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * The numbered walkthrough for registering a team's own GitHub App.
 *
 * This must stay in step with the docs-site guide (issue #486) — both are
 * written against the same checklist so they cannot disagree. The ordering
 * matters more than it looks: the webhook URL only exists after the
 * credentials are saved, so the guide has to tell people to leave the webhook
 * blank on GitHub and come back for it. A setup that stops after step 3 looks
 * complete and silently drops every installation event.
 */

/** Repository permissions the App needs, with the reason for each. */
const REQUIRED_PERMISSIONS: { name: string; access: string; why: string }[] = [
  {
    name: 'Contents',
    access: 'Read-only',
    why: 'Reads blueprint and configuration files out of your repositories when importing.',
  },
  {
    name: 'Metadata',
    access: 'Read-only',
    why: 'Mandatory for every GitHub App — it is what lets the App list the repositories an installation can see. GitHub selects it automatically.',
  },
]

/** Webhook events the App must subscribe to, with the reason for each. */
const REQUIRED_EVENTS: { name: string; why: string }[] = [
  {
    name: 'Installation',
    why: 'Keeps the connection state accurate when the App is installed, uninstalled, or suspended — without polling.',
  },
  {
    name: 'Installation repositories',
    why: 'Tells VibeXP when repositories are added to or removed from an existing installation.',
  },
]

export interface GitHubAppSetupGuideProps {
  /**
   * Pre-fills the organization the App is created under. The field stays
   * editable — this is only a starting value.
   */
  defaultOrganization?: string
}

export function GitHubAppSetupGuide({
  defaultOrganization = '',
}: GitHubAppSetupGuideProps) {
  // The create-App URL differs for a personal account and an org, and there is
  // no way to know which one someone wants: asking is cheaper than sending them
  // to the wrong page and letting them work out why the App is owned by the
  // wrong account (which only becomes visible much later, when that account
  // goes away).
  const [organization, setOrganization] = useState(defaultOrganization)
  const trimmedOrg = organization.trim()
  const createUrl = trimmedOrg
    ? `https://github.com/organizations/${encodeURIComponent(trimmedOrg)}/settings/apps/new`
    : 'https://github.com/settings/apps/new'

  return (
    <div className="space-y-6 text-sm">
      <ol className="space-y-5">
        <li>
          <p className="font-medium">1. Create a GitHub App</p>
          <p className="text-muted-foreground mt-1">
            Own the App at the organization level when the repositories belong
            to an org — an App owned by a personal account keeps working only as
            long as that account does. Leave the{' '}
            <strong>Webhook URL blank</strong> for now; you will come back for
            it in step 3.
          </p>
          <div className="mt-2 space-y-1">
            <Label htmlFor="github-app-org" className="text-xs">
              Organization (optional)
            </Label>
            <Input
              id="github-app-org"
              value={organization}
              onChange={event => {
                setOrganization(event.target.value)
              }}
              placeholder="acme-inc — leave blank for your personal account"
              className="max-w-sm"
            />
          </div>
          <Button asChild variant="outline" size="sm" className="mt-2">
            <a href={createUrl} target="_blank" rel="noopener noreferrer">
              {trimmedOrg
                ? `Create App in ${trimmedOrg}`
                : 'Create App on GitHub'}
              <ExternalLink className="ml-2 h-3.5 w-3.5" />
            </a>
          </Button>
          <p className="text-muted-foreground mt-2 text-xs">
            If your organization restricts App creation, ask an owner to create
            it and hand you the credentials, or create it on a personal account
            and install it on the org — installing is a separate permission from
            creating.
          </p>

          <p className="mt-3 font-medium">Permissions to grant</p>
          <ul className="mt-1 space-y-1">
            {REQUIRED_PERMISSIONS.map(permission => (
              <li key={permission.name} className="text-muted-foreground">
                <span className="text-foreground font-mono text-xs">
                  {permission.name}: {permission.access}
                </span>{' '}
                — {permission.why}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground mt-1 text-xs">
            Nothing else. VibeXP never writes to your repositories through this
            App.
          </p>

          <p className="mt-3 font-medium">Events to subscribe to</p>
          <ul className="mt-1 space-y-1">
            {REQUIRED_EVENTS.map(event => (
              <li key={event.name} className="text-muted-foreground">
                <span className="text-foreground font-mono text-xs">
                  {event.name}
                </span>{' '}
                — {event.why}
              </li>
            ))}
          </ul>

          <p className="mt-3 font-medium">User authorization</p>
          <p className="text-muted-foreground mt-1">
            Enable{' '}
            <strong>
              “Request user authorization (OAuth) during installation”
            </strong>
            . This is not optional: connecting an installation exchanges
            GitHub’s authorization code for a user token to check that the
            person can actually administer it. Without it the connect step fails
            closed.
          </p>
        </li>

        <li>
          <p className="font-medium">2. Copy the credentials into VibeXP</p>
          <p className="text-muted-foreground mt-1">
            From the App’s settings page you need five values:{' '}
            <strong>App ID</strong>, <strong>App slug</strong> (the last segment
            of <code className="text-xs">github.com/apps/&lt;slug&gt;</code>),{' '}
            <strong>Client ID</strong>, a generated{' '}
            <strong>Client secret</strong>, and a generated{' '}
            <strong>private key</strong> (the downloaded{' '}
            <code className="text-xs">.pem</code>). The key is accepted as raw
            PEM or base64 — you do not need to convert anything.
          </p>
        </li>

        <li>
          <p className="font-medium">3. Wire the webhook, then verify</p>
          <p className="text-muted-foreground mt-1">
            Saving reveals the <strong>webhook URL</strong> and a{' '}
            <strong>webhook secret VibeXP generates for you</strong> — shown{' '}
            <strong>once</strong>. Paste both into the App’s Webhook settings on
            GitHub, then come back and click <strong>Verify</strong>.
          </p>
          <p className="text-muted-foreground mt-1 text-xs">
            Do not stop after saving: credentials stored with the webhook
            unwired look completely fine here and silently drop every
            installation event.
          </p>
        </li>

        <li>
          <p className="font-medium">4. Install the App</p>
          <p className="text-muted-foreground mt-1">
            Install it on the account or organization whose repositories you
            want, choose the repositories, and complete the redirect back here.
          </p>
        </li>
      </ol>
    </div>
  )
}
