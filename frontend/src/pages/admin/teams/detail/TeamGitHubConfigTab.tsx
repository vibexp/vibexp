import { useCallback } from 'react'

import { formatDateTime } from '@/lib/time'
import type { AdminTeamGitHubConfig } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import { ConfigField, ConfigGrid, OnOffBadge, SecretState } from './ConfigField'
import { displayValue } from './teamConfigFormat'
import { useAdminTeamSection } from './useAdminTeamSection'

function AppConfig({
  app,
}: Readonly<{ app: AdminTeamGitHubConfig['app_config'] }>) {
  if (!app) {
    return (
      <p className="text-muted-foreground text-sm">
        No GitHub App configured for this team.
      </p>
    )
  }
  return (
    <ConfigGrid>
      <ConfigField label="App ID">{app.app_id}</ConfigField>
      <ConfigField label="App slug">{app.app_slug}</ConfigField>
      <ConfigField label="Client ID">{app.client_id}</ConfigField>
      <ConfigField label="Private key">
        <SecretState configured={app.has_private_key} />
      </ConfigField>
      <ConfigField label="Client secret">
        <SecretState configured={app.has_client_secret} />
      </ConfigField>
      <ConfigField label="Webhook secret">
        <SecretState configured={app.has_webhook_secret} />
      </ConfigField>
      <ConfigField label="Webhook URL">
        <SecretState configured={app.webhook_configured} />
      </ConfigField>
    </ConfigGrid>
  )
}

function Installation({
  installation,
}: Readonly<{ installation: AdminTeamGitHubConfig['installation'] }>) {
  if (!installation.installed) {
    return <p className="text-muted-foreground text-sm">Not installed.</p>
  }
  return (
    <ConfigGrid>
      <ConfigField label="Account">
        {displayValue(installation.account_login)}
      </ConfigField>
      <ConfigField label="Installation ID">
        {displayValue(installation.installation_id)}
      </ConfigField>
      <ConfigField label="Suspended">
        <OnOffBadge
          on={installation.suspended}
          onLabel="Suspended"
          offLabel="Active"
        />
      </ConfigField>
      <ConfigField label="Installed at">
        {installation.installed_at
          ? formatDateTime(installation.installed_at)
          : '—'}
      </ConfigField>
    </ConfigGrid>
  )
}

/** The team's GitHub App registration and installation (read-only). */
export function TeamGitHubConfigTab({ teamId }: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamGitHubConfig(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load GitHub integration'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load GitHub integration"
    >
      {data && (
        <>
          <ConfigSection title="GitHub App">
            <AppConfig app={data.app_config} />
          </ConfigSection>
          <ConfigSection title="Installation">
            <Installation installation={data.installation} />
          </ConfigSection>
        </>
      )}
    </AdminConfigPanel>
  )
}
