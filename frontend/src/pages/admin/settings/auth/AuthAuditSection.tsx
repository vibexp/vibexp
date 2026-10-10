import { useCallback, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { AdminInstanceSettingsAuditEntry } from '@/services/adminService'
import type { AdminInstanceSettingsAuditParams } from '@/services/adminSettingsService'
import { authSettingsService } from '@/services/authSettingsService'

import { InstanceSettingsAuditList } from '../InstanceSettingsAuditList'
import {
  AUTH_AUDIT_SECTIONS,
  authAuditActorLabel,
  authAuditChanges,
  type AuthAuditSection as Section,
  clientSecretChange,
  madeFromCli,
} from './authSettingsAudit'

function EntryBadges({
  entry,
}: Readonly<{ entry: AdminInstanceSettingsAuditEntry }>) {
  const secret = clientSecretChange(entry)
  return (
    <>
      {secret && (
        <Badge variant="secondary" className="font-normal">
          {secret === 'changed'
            ? 'Client secret changed'
            : 'Client secret unchanged'}
        </Badge>
      )}
      {madeFromCli(entry) && (
        <Badge variant="outline" className="font-normal">
          via CLI
        </Badge>
      )}
    </>
  )
}

function SettingHistory({
  section,
  refreshKey,
}: Readonly<{ section: Section; refreshKey: number }>) {
  const { setting } = section
  const fetchPage = useCallback(
    (params: AdminInstanceSettingsAuditParams) =>
      authSettingsService.listAudit(setting, params),
    [setting]
  )
  return (
    <InstanceSettingsAuditList
      fetchPage={fetchPage}
      describeChanges={entry => authAuditChanges(entry, section)}
      entryBadge={entry => <EntryBadges entry={entry} />}
      actionLabel={section.actionLabel}
      actorLabel={authAuditActorLabel}
      description={section.description}
      entryTestId={`auth-audit-entry-${setting}`}
      refreshKey={refreshKey}
    />
  )
}

/**
 * Audit history of the authentication settings (#1239): one change history
 * per setting — providers, allowlist, instance admins and setup mode — behind
 * tabs. `refreshKey` reloads the open history after a change on the page.
 */
export function AuthAuditSection({
  refreshKey,
}: Readonly<{ refreshKey: number }>) {
  const [setting, setSetting] = useState<Section['setting']>('auth_providers')
  return (
    <Tabs
      value={setting}
      onValueChange={value => {
        setSetting(value as Section['setting'])
      }}
      data-testid="auth-audit-section"
    >
      <TabsList aria-label="Authentication settings history">
        {AUTH_AUDIT_SECTIONS.map(section => (
          <TabsTrigger key={section.setting} value={section.setting}>
            {section.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {AUTH_AUDIT_SECTIONS.map(section => (
        <TabsContent key={section.setting} value={section.setting}>
          <SettingHistory section={section} refreshKey={refreshKey} />
        </TabsContent>
      ))}
    </Tabs>
  )
}
