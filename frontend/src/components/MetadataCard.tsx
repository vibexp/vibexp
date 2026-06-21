import { type ReactNode } from 'react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'

/**
 * A single label/value row used inside the Additional data card.
 */
function MetaRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right">{children}</span>
    </div>
  )
}

/**
 * Converts a snake_case or kebab-case key to Sentence case.
 * Examples: "active_count" -> "Active count", "my-key" -> "My key"
 */
function formatKey(key: string): string {
  const spaced = key.replace(/[_-]/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

/**
 * Renders a single metadata value according to its type:
 * - Primitives: rendered as text (booleans as Yes/No, numbers via toLocaleString)
 * - null / undefined: rendered as em-dash in muted color
 * - Objects / arrays: rendered as a small JSON code block
 */
function MetaValue({ value }: { value: unknown }) {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground">—</span>
  }
  if (typeof value === 'boolean') {
    return <>{value ? 'Yes' : 'No'}</>
  }
  if (typeof value === 'number') {
    return <>{value.toLocaleString()}</>
  }
  if (typeof value === 'string') {
    return <>{value}</>
  }
  // objects / arrays
  let serialized: string
  try {
    serialized = JSON.stringify(value)
  } catch {
    serialized = '[unserializable]'
  }
  return (
    <code className="bg-muted text-muted-foreground block overflow-x-auto rounded px-1 font-mono text-xs break-all">
      {serialized}
    </code>
  )
}

interface AdditionalDataCardProps {
  data: Record<string, unknown>
}

/**
 * Renders a Record<string, unknown> as key-value rows in a Card.
 * Returns null when the record is empty.
 */
export function AdditionalDataCard({ data }: AdditionalDataCardProps) {
  const entries = Object.entries(data)
  if (entries.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle as="h3" className="text-sm">
          Additional data
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        {entries.map(([key, value], index) => (
          <div key={key}>
            {index > 0 && <Separator className="mb-2" />}
            <MetaRow label={formatKey(key)}>
              <MetaValue value={value} />
            </MetaRow>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
