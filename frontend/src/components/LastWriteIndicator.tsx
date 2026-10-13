import {
  type Channel,
  CHANNEL_FILL,
  CHANNEL_LABEL,
} from '@/components/channelSeries'
import { marksLastWrite } from '@/components/lastWrite'
import { cn } from '@/lib/utils'

/** The channel's dot, as the dashboard figures draw it. The label says it too. */
function ChannelDot({
  channel,
  className,
}: Readonly<{ channel: Channel; className?: string }>) {
  return (
    <span
      className={cn('size-2 shrink-0 rounded-full', className)}
      style={{ backgroundColor: CHANNEL_FILL[channel] }}
      aria-hidden="true"
    />
  )
}

/**
 * The channel a resource was last written through (#1452): MCP, the CLI, the
 * API or the web app, with the name the MCP client gave itself when there is
 * one. Labels and colours are `channelSeries`' own, so a channel reads the same
 * here as in the dashboard figures.
 *
 * Callers pass `last_write_source` and `last_write_assistant_name` straight
 * through. `null` means no write was recorded and is never shown as a channel.
 *
 * - `inline`, for a row's meta line: "via MCP · Claude Code". **Renders nothing
 *   for a web write and for `null`** ({@link marksLastWrite}), so only what an
 *   AI tool or a script wrote is marked and a plain row costs no markup. It is
 *   inline content: the line it sits in, or `className`, does the truncating.
 * - `full`, for a details row: always a value. "MCP · Claude Code", "CLI",
 *   "API", "Web" or "Not recorded".
 *
 * The words carry the meaning; the dot is decoration. The assistant name is a
 * label of up to 30 characters that the client chose, so it is the part that
 * truncates, and it keeps its full text in `title`.
 */
export function LastWriteIndicator({
  source,
  assistantName,
  variant,
  className,
}: Readonly<{
  source: Channel | null | undefined
  assistantName?: string | null
  variant: 'inline' | 'full'
  className?: string
}>) {
  const name = source && assistantName ? assistantName : null

  if (variant === 'inline') {
    if (!marksLastWrite(source)) return null
    return (
      <span className={className} data-testid="last-write-indicator">
        <ChannelDot
          channel={source}
          className="mr-1.5 inline-block align-baseline"
        />
        via {CHANNEL_LABEL[source]}
        {name && (
          <>
            {' · '}
            <span title={name}>{name}</span>
          </>
        )}
      </span>
    )
  }

  if (!source) {
    return (
      <span
        className={cn('text-muted-foreground font-normal', className)}
        data-testid="last-write-indicator"
      >
        Not recorded
      </span>
    )
  }
  return (
    <span
      className={cn('inline-flex min-w-0 items-center gap-1.5', className)}
      data-testid="last-write-indicator"
    >
      <ChannelDot channel={source} />
      <span className="shrink-0">{CHANNEL_LABEL[source]}</span>
      {name && (
        <>
          <span aria-hidden="true" className="text-muted-foreground">
            ·
          </span>
          <span className="truncate" title={name}>
            {name}
          </span>
        </>
      )}
    </span>
  )
}
