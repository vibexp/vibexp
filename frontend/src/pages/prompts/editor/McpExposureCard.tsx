import { useWatch } from 'react-hook-form'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

export interface McpExposureCardProps {
  value: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
}

/**
 * The prompt form's `mcp-exposure` extension slot.
 *
 * Only prompts are MCP-exposable, and the flag is a fact about the share rather
 * than a field of the prompt — so it is a page-owned toggle rather than a
 * descriptor field (there is no boolean control kind, deliberately).
 *
 * The switch appears only once the prompt is `published`, which is the one
 * thing the slot needs from the form itself. `ResourceFormPage` renders its
 * extensions INSIDE the `FormProvider`, so `useWatch` reads the live status
 * without the page having to mirror the form's state or the shared component
 * having to grow a values callback.
 */
export function McpExposureCard({
  value,
  onChange,
  disabled = false,
}: Readonly<McpExposureCardProps>) {
  // Typed explicitly: `useWatch` over a `Record<string, unknown>` form has no
  // per-field type to infer, so its return is `any` without one.
  const status = useWatch<Record<string, unknown>>({ name: 'status' })
  if (status !== 'published') return null

  return (
    <Card data-testid="prompt-mcp-exposure-card">
      <CardHeader>
        <CardTitle className="text-sm">MCP</CardTitle>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-4">
        <div>
          <Label htmlFor="prompt-mcp-expose" className="text-sm font-medium">
            Available in MCP
          </Label>
          <p className="text-muted-foreground text-xs">
            Allow AI assistants to discover this prompt via MCP.
          </p>
        </div>
        <Switch
          id="prompt-mcp-expose"
          checked={value}
          disabled={disabled}
          data-testid="prompt-mcp-expose"
          onCheckedChange={onChange}
        />
      </CardContent>
    </Card>
  )
}
