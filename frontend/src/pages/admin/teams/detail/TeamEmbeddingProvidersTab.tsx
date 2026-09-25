import { useCallback } from 'react'

import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type {
  AdminEmbeddingProvider,
  AdminTeamEmbeddingProvidersConfig,
} from '@/services/adminService'
import { adminService } from '@/services/adminService'

import { AdminConfigPanel, ConfigSection } from './AdminConfigPanel'
import { ConfigField, ConfigGrid } from './ConfigField'
import { ProviderTable } from './ProviderTable'
import { displayValue } from './teamConfigFormat'
import { useAdminTeamSection } from './useAdminTeamSection'

function EmbeddingDetails({
  provider,
}: Readonly<{ provider: AdminEmbeddingProvider }>) {
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-2 text-xs">
      <dt className="text-muted-foreground">Chunk</dt>
      <dd>
        {provider.chunk_size} / {provider.chunk_overlap} overlap
      </dd>
      <dt className="text-muted-foreground">Concurrency</dt>
      <dd>{provider.concurrency}</dd>
      <dt className="text-muted-foreground">Query prefix</dt>
      <dd className="break-all">{displayValue(provider.query_prefix)}</dd>
      <dt className="text-muted-foreground">Document prefix</dt>
      <dd className="break-all">{displayValue(provider.document_prefix)}</dd>
    </dl>
  )
}

function Coverage({
  coverage,
}: Readonly<{ coverage: AdminTeamEmbeddingProvidersConfig['coverage'] }>) {
  return (
    <ConfigSection title="Embedding coverage">
      <ConfigGrid>
        <ConfigField label="Active model">
          {coverage.has_active_provider
            ? displayValue(coverage.active_model)
            : 'No active provider'}
        </ConfigField>
      </ConfigGrid>
      {coverage.items.length > 0 && (
        <Card className="overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="h-9 text-xs font-medium">Type</TableHead>
                <TableHead className="h-9 text-right text-xs font-medium">
                  Total
                </TableHead>
                <TableHead className="h-9 text-right text-xs font-medium">
                  Embedded
                </TableHead>
                <TableHead className="h-9 text-right text-xs font-medium">
                  Pending
                </TableHead>
                <TableHead className="h-9 text-right text-xs font-medium">
                  Coverage
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {coverage.items.map(item => (
                <TableRow key={item.entity_type} data-testid="coverage-row">
                  <TableCell className="py-2 text-sm">
                    {item.entity_type}
                  </TableCell>
                  <TableCell className="py-2 text-right text-sm tabular-nums">
                    {item.total}
                  </TableCell>
                  <TableCell className="py-2 text-right text-sm tabular-nums">
                    {item.embedded}
                  </TableCell>
                  <TableCell className="py-2 text-right text-sm tabular-nums">
                    {item.pending}
                  </TableCell>
                  <TableCell className="py-2 text-right text-sm tabular-nums">
                    {item.embedded_percent}%
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </ConfigSection>
  )
}

/** The team's own embedding providers and its embedding coverage (read-only). */
export function TeamEmbeddingProvidersTab({
  teamId,
}: Readonly<{ teamId: string }>) {
  const load = useCallback(
    () => adminService.getTeamEmbeddingProviders(teamId),
    [teamId]
  )
  const { data, loading, error } = useAdminTeamSection(
    load,
    'Failed to load embedding providers'
  )

  return (
    <AdminConfigPanel
      loading={loading}
      error={error}
      errorTitle="Failed to load embedding providers"
    >
      {data && (
        <>
          {data.providers.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="config-empty"
            >
              No embedding providers configured for this team.
            </p>
          ) : (
            <ProviderTable
              providers={data.providers}
              extraHeader="Embedding"
              extra={p => <EmbeddingDetails provider={p} />}
            />
          )}
          <Coverage coverage={data.coverage} />
        </>
      )}
    </AdminConfigPanel>
  )
}
