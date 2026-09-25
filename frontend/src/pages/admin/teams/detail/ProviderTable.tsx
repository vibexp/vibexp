import type { ReactNode } from 'react'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AdminModelProvider } from '@/services/adminService'

import { SecretState } from './ConfigField'
import { displayValue } from './teamConfigFormat'

/** The redacted fields model and embedding providers share. */
export type ProviderRow = Pick<
  AdminModelProvider,
  | 'id'
  | 'name'
  | 'provider_type'
  | 'model'
  | 'base_url'
  | 'is_default'
  | 'has_api_key'
  | 'configuration_keys'
>

/** Configuration key names as chips — the values are never sent. */
function ConfigurationKeys({ keys }: Readonly<{ keys: string[] }>) {
  if (keys.length === 0) {
    return <span className="text-muted-foreground">—</span>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {keys.map(key => (
        <Badge key={key} variant="outline" className="font-mono font-normal">
          {key}
        </Badge>
      ))}
    </div>
  )
}

/**
 * A read-only provider table. Rendered field by field — a provider object is
 * never spread or stringified, so a key the server adds cannot surface here.
 * `extra` adds per-provider details (embedding chunking) as one more column.
 */
export function ProviderTable<T extends ProviderRow>({
  providers,
  extraHeader,
  extra,
}: Readonly<{
  providers: T[]
  extraHeader?: string
  extra?: (provider: T) => ReactNode
}>) {
  return (
    <Card className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/40 hover:bg-muted/40">
            <TableHead className="h-9 text-xs font-medium">Name</TableHead>
            <TableHead className="h-9 text-xs font-medium">Type</TableHead>
            <TableHead className="h-9 text-xs font-medium">Model</TableHead>
            <TableHead className="h-9 text-xs font-medium">Base URL</TableHead>
            <TableHead className="h-9 text-xs font-medium">API key</TableHead>
            <TableHead className="h-9 text-xs font-medium">
              Configuration
            </TableHead>
            {extra && (
              <TableHead className="h-9 text-xs font-medium">
                {extraHeader}
              </TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {providers.map(p => (
            <TableRow key={p.id} data-testid="provider-row">
              <TableCell className="py-3 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <span>{p.name}</span>
                  {p.is_default && (
                    <Badge variant="secondary" className="font-normal">
                      Default
                    </Badge>
                  )}
                </div>
              </TableCell>
              <TableCell className="py-3 text-sm">{p.provider_type}</TableCell>
              <TableCell className="py-3 text-sm">{p.model}</TableCell>
              <TableCell className="py-3 text-sm break-all">
                {displayValue(p.base_url)}
              </TableCell>
              <TableCell className="py-3 text-sm whitespace-nowrap">
                <SecretState configured={p.has_api_key} />
              </TableCell>
              <TableCell className="py-3 text-sm">
                <ConfigurationKeys keys={p.configuration_keys} />
              </TableCell>
              {extra && (
                <TableCell className="py-3 text-sm">{extra(p)}</TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}
