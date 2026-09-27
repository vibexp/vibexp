import { useWatch } from 'react-hook-form'

import { Switch } from '@/components/ui/switch'

export interface McpExposureSwitchProps {
  value: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
}

/**
 * The prompt form's `mcp-exposure` extension slot: the value of the Metadata
 * section's MCP row, where the reading page shows "Exposed" / "Not exposed" —
 * so the create and edit pages add no card or heading of their own (#1180).
 * Until the prompt is published there is nothing to toggle, and the row says
 * what the reading page would.
 *
 * Only prompts are MCP-exposable, and the flag is a fact about the share rather
 * than a field of the prompt — so it is a page-owned toggle rather than a
 * descriptor field. It renders INSIDE the `FormProvider`, so `useWatch` reads
 * the live status without the page having to mirror the form's state.
 */
export function McpExposureSwitch({
  value,
  onChange,
  disabled = false,
}: Readonly<McpExposureSwitchProps>) {
  const status = useWatch<Record<string, unknown>>({ name: 'status' })
  if (status !== 'published') {
    return (
      <span
        className="text-muted-foreground font-normal"
        title="Publish the prompt to make it available in MCP"
      >
        Not exposed
      </span>
    )
  }
  return (
    <Switch
      checked={value}
      disabled={disabled}
      aria-label="Available in MCP"
      data-testid="prompt-mcp-expose"
      onCheckedChange={onChange}
    />
  )
}
