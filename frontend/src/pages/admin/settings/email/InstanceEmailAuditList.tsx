import { Badge } from '@/components/ui/badge'
import {
  type AdminInstanceEmailAuditParams,
  type AdminInstanceSettingsAuditEntry,
  adminService,
} from '@/services/adminService'

import { InstanceSettingsAuditList } from '../InstanceSettingsAuditList'
import { auditFieldChanges, credentialChange } from './instanceEmailAudit'

const fetchEmailAudit = (params: AdminInstanceEmailAuditParams) =>
  adminService.listInstanceEmailSettingsAudit(params)

function CredentialBadge({
  entry,
}: Readonly<{ entry: AdminInstanceSettingsAuditEntry }>) {
  const credential = credentialChange(entry)
  if (!credential) return null
  return (
    <Badge variant="secondary" className="font-normal">
      {credential === 'changed' ? 'Credential changed' : 'Credential unchanged'}
    </Badge>
  )
}

/**
 * The instance email provider's change history (#1191). `refreshKey` reloads
 * from the first page — the page bumps it after each save or removal.
 */
export function InstanceEmailAuditList({
  refreshKey,
}: Readonly<{ refreshKey: number }>) {
  return (
    <InstanceSettingsAuditList
      fetchPage={fetchEmailAudit}
      describeChanges={auditFieldChanges}
      entryBadge={entry => <CredentialBadge entry={entry} />}
      description="Every change to the instance email provider. Credentials are never recorded — only whether one changed."
      entryTestId="instance-email-audit-entry"
      refreshKey={refreshKey}
    />
  )
}
