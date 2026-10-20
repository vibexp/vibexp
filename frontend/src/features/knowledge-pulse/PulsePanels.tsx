import { Bot, User } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Area, AreaChart, ResponsiveContainer, YAxis } from 'recharts'

import { cn } from '@/lib/utils'

import { AnimatedNumber } from './AnimatedNumber'
import { describeEvent, shortAge } from './describeEvent'
import type { StatsSample } from './pulseState'
import {
  EVENT_KIND_COLOR,
  EVENT_KIND_LABEL,
  RESOURCE_TYPE_COLOR,
  RESOURCE_TYPE_LABEL,
} from './pulseTheme'
import type { PulseEvent, PulseEventKind, PulseNode, PulseStats } from './types'

function useNow(everyMs: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => {
      setNow(Date.now())
    }, everyMs)
    return () => {
      clearInterval(id)
    }
  }, [everyMs])
  return now
}

export function KindDot({
  kind,
  className,
}: Readonly<{ kind: PulseEventKind; className?: string }>) {
  return (
    <span
      aria-hidden="true"
      className={cn('inline-block size-2 shrink-0 rounded-full', className)}
      style={{ backgroundColor: EVENT_KIND_COLOR[kind] }}
    />
  )
}

/** Live, newest-first stream of what just happened. */
export function EventFeed({
  events,
  onOpen,
  className,
}: Readonly<{
  events: readonly PulseEvent[]
  onOpen: (ref: PulseEvent['resource']) => void
  className?: string
}>) {
  const now = useNow(1000)
  return (
    <div className={cn('flex min-h-0 flex-col rounded-lg border', className)}>
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-sm font-semibold">Live activity</h2>
        <span className="text-muted-foreground text-xs">
          {events.length > 0 ? `${String(events.length)} recent` : ''}
        </span>
      </div>
      {events.length === 0 ? (
        <p className="text-muted-foreground px-4 py-8 text-center text-sm">
          Quiet for now. Press play and events stream in here.
        </p>
      ) : (
        <ol
          className="min-h-0 flex-1 divide-y overflow-y-auto"
          aria-live="polite"
        >
          {events.map((event, i) => {
            const s = describeEvent(event)
            return (
              <li
                key={event.id}
                className={cn(
                  'flex gap-3 px-4 py-2.5 text-sm',
                  i === 0 &&
                    'animate-in fade-in slide-in-from-top-1 duration-500'
                )}
              >
                <span className="bg-muted mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full">
                  {event.actor.type === 'agent' ? (
                    <Bot className="size-3.5" aria-hidden="true" />
                  ) : (
                    <User className="size-3.5" aria-hidden="true" />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="line-clamp-3 leading-snug">
                    <span className="font-medium">{event.actor.name}</span>{' '}
                    <span className="text-muted-foreground">{s.verb}</span>{' '}
                    <button
                      type="button"
                      className="hover:text-primary text-left font-medium underline-offset-2 hover:underline"
                      onClick={() => {
                        onOpen(event.resource)
                      }}
                    >
                      {s.object}
                    </button>
                    {s.target && (
                      <>
                        <span className="text-muted-foreground"> to </span>
                        <span className="font-medium">{s.target}</span>
                      </>
                    )}
                  </p>
                  <p className="text-muted-foreground mt-0.5 flex items-center gap-1.5 text-xs">
                    <KindDot kind={event.kind} />
                    {EVENT_KIND_LABEL[event.kind]}
                    {s.suffix && <span>· {s.suffix}</span>}
                    <span className="ml-auto tabular-nums">
                      {shortAge(event.at, now)} ago
                    </span>
                  </p>
                </div>
              </li>
            )
          })}
        </ol>
      )}
    </div>
  )
}

function Sparkline({
  data,
  dataKey,
  color,
}: Readonly<{
  data: readonly StatsSample[]
  dataKey: keyof StatsSample
  color: string
}>) {
  if (data.length < 2) return <div className="h-10" />
  const id = `spark-${dataKey}`
  return (
    <div className="h-10" aria-hidden="true">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart
          data={[...data]}
          margin={{ top: 2, right: 0, bottom: 0, left: 0 }}
        >
          <defs>
            <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.35} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          {/* Scale to the session's range: today's totals barely move in
              relative terms, so a zero baseline would draw a flat line. */}
          <YAxis hide domain={['dataMin', 'dataMax']} />
          <Area
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            strokeWidth={1.5}
            fill={`url(#${id})`}
            isAnimationActive
            animationDuration={900}
            animationEasing="ease-out"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

const TILES: readonly {
  key: keyof PulseStats
  sample: keyof StatsSample
  label: string
  hint: string
  color: string
}[] = [
  {
    key: 'reads',
    sample: 'reads',
    label: 'Reads today',
    hint: 'Knowledge fetched before a task',
    color: EVENT_KIND_COLOR.read,
  },
  {
    key: 'writes',
    sample: 'writes',
    label: 'Writes today',
    hint: 'Created or updated',
    color: EVENT_KIND_COLOR.write,
  },
  {
    key: 'links',
    sample: 'links',
    label: 'New links today',
    hint: 'Relations recorded',
    color: EVENT_KIND_COLOR.link,
  },
  {
    key: 'compounding_score',
    sample: 'compounding',
    label: 'Compounding score',
    hint: 'Links + supersedes + updates to linked items',
    color: 'var(--foreground)',
  },
]

/** Today's totals, each with a sparkline of how it moved this session. */
export function StatTiles({
  stats,
  history,
  tweenMs,
  reducedMotion,
}: Readonly<{
  stats: PulseStats | null
  history: readonly StatsSample[]
  /** How long each count-up takes; the page passes the poll interval. */
  tweenMs: number
  reducedMotion: boolean
}>) {
  const first = history[0] as StatsSample | undefined
  return (
    <dl className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      {TILES.map(tile => {
        const value = stats?.[tile.key] ?? 0
        const delta = first ? value - first[tile.sample] : 0
        return (
          <div key={tile.key} className="rounded-lg border px-4 pt-3 pb-2">
            <dt className="text-muted-foreground flex items-center gap-2 text-xs font-medium">
              <span
                aria-hidden="true"
                className="inline-block size-2 rounded-full"
                style={{ backgroundColor: tile.color }}
              />
              {tile.label}
            </dt>
            <dd className="mt-1 flex items-baseline gap-2">
              <span className="text-2xl font-semibold tabular-nums">
                {stats ? (
                  <AnimatedNumber
                    value={value}
                    durationMs={tweenMs}
                    disabled={reducedMotion}
                  />
                ) : (
                  '—'
                )}
              </span>
              {delta > 0 && (
                <span
                  className="text-xs font-medium tabular-nums"
                  style={{ color: tile.color }}
                >
                  +
                  <AnimatedNumber
                    value={delta}
                    durationMs={tweenMs}
                    disabled={reducedMotion}
                  />{' '}
                  this session
                </span>
              )}
            </dd>
            <dd className="text-muted-foreground text-[11px]">{tile.hint}</dd>
            <dd>
              <Sparkline
                data={history}
                dataKey={tile.sample}
                color={tile.color}
              />
            </dd>
          </div>
        )
      })}
    </dl>
  )
}

/** The most-touched items since the page opened. */
export function TopItems({
  nodes,
  touches,
  onOpen,
}: Readonly<{
  nodes: ReadonlyMap<string, PulseNode>
  touches: ReadonlyMap<string, number>
  onOpen: (node: PulseNode) => void
}>) {
  const ranked = [...touches.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 8)
    .flatMap(([id, count]) => {
      const node = nodes.get(id)
      return node ? [{ node, count }] : []
    })
  const max = ranked[0]?.count ?? 1
  return (
    <div className="rounded-lg border">
      <div className="border-b px-4 py-3">
        <h2 className="text-sm font-semibold">Most touched</h2>
      </div>
      {ranked.length === 0 ? (
        <p className="text-muted-foreground px-4 py-6 text-center text-sm">
          Nothing yet.
        </p>
      ) : (
        <ol className="space-y-1 p-2">
          {ranked.map(({ node, count }, i) => (
            <li key={node.id}>
              <button
                type="button"
                onClick={() => {
                  onOpen(node)
                }}
                className="hover:bg-muted relative flex w-full items-center gap-3 overflow-hidden rounded-md px-2 py-1.5 text-left text-sm"
              >
                <span
                  aria-hidden="true"
                  className="absolute inset-y-0 left-0 opacity-10 transition-[width] duration-700"
                  style={{
                    width: `${String((count / max) * 100)}%`,
                    backgroundColor: RESOURCE_TYPE_COLOR[node.type],
                  }}
                />
                <span className="text-muted-foreground relative w-4 text-xs tabular-nums">
                  {i + 1}
                </span>
                <span className="relative min-w-0 flex-1 truncate font-medium">
                  {node.title}
                </span>
                <span className="text-muted-foreground relative text-xs">
                  {RESOURCE_TYPE_LABEL[node.type]}
                </span>
                <span className="relative w-8 text-right text-xs font-semibold tabular-nums">
                  {count}
                </span>
              </button>
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
