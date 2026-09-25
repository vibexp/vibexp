import { useCallback } from 'react'

import { Badge } from '@/components/ui/badge'
import { formatDateTime } from '@/lib/time'
import type { AdminTeamEmailProviderConfig } from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import {
  ConfigField,
  ConfigGrid,
  SecretState,
  SourceBadge,
} from './ConfigField'
import { displayValue, emailStatusMeta } from './teamConfigFormat'
import { useAdminTeamSection } from './useAdminTeamSection'

/**
 * The non-secret settings for the provider's type. Field by field: the SMTP
 * username is not in the payload, and nothing here would show it if it were.
 */
function ProviderSettings({
  settings,
}: Readonly<{ settings: AdminTeamEmailProviderConfig['settings'] }>) {
  if (settings?.smtp) {
    return (
      <>
        <ConfigField label="SMTP host">{settings.smtp.host}</ConfigField>
        <ConfigField label="SMTP port">{settings.smtp.port}</ConfigField>
      </>
    )
  }
  if (settings?.mailgun) {
    return (
      <>
        <ConfigField label="Mailgun domain">
          {settings.mailgun.domain}
        </ConfigField>
        <ConfigField label="Mailgun base URL">
          {displayValue(settings.mailgun.base_url)}
        </ConfigField>
      </>
    )
  }
  if (settings?.postmark) {
    return (
      <ConfigField label="Postmark stream">
        {settings.postmark.message_stream}
      </ConfigField>
    )
  }
  return null
}

function EmailStatus({
  config,
}: Readonly<{ config: AdminTeamEmailProviderConfig }>) {
  const meta = emailStatusMeta(config.status)
  return (
    <ConfigGrid>
      <ConfigField label="Status">
        <Badge variant={meta.variant} className="font-normal">
          {meta.label}
        </Badge>
      </ConfigField>
      <ConfigField label="Last success">
        {config.last_success_at ? formatDateTime(config.last_success_at) : '—'}
      </ConfigField>
      <ConfigField label="Last error">
        {config.last_error_at ? formatDateTime(config.last_error_at) : '—'}
      </ConfigField>
    </ConfigGrid>
  )
}

/** The email provider in effect for the team and its health (read-only). */
export function TeamEmailConfigTab({ teamId }: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamEmailProvider(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load email provider'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load email provider"
    >
      {data && (
        <ConfigSection
          title="Email provider"
          aside={<SourceBadge source={data.source} />}
        >
          <ConfigGrid>
            <ConfigField label="Effective from address">
              {data.effective_from_address}
            </ConfigField>
            {data.source === 'team' && (
              <>
                <ConfigField label="Provider type">
                  {displayValue(data.provider_type)}
                </ConfigField>
                <ConfigField label="From">
                  {displayValue(data.from_name)}{' '}
                  {data.from_address && `<${data.from_address}>`}
                </ConfigField>
                <ConfigField label="Reply-to">
                  {displayValue(data.reply_to)}
                </ConfigField>
                <ProviderSettings settings={data.settings} />
                <ConfigField label="Credential">
                  <SecretState configured={data.has_secret} />
                </ConfigField>
              </>
            )}
          </ConfigGrid>
          <EmailStatus config={data} />
        </ConfigSection>
      )}
    </AdminConfigPanel>
  )
}
