import { Maximize2, Minimize2, Pause, Play } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'

import { PageHeader } from '@/components/PageHeader'
import {
  SegmentedControl,
  type SegmentedOption,
} from '@/components/SegmentedControl'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { capNodes } from '@/features/knowledge-pulse/layout'
import {
  EventFeed,
  KindDot,
  StatTiles,
  TopItems,
} from '@/features/knowledge-pulse/PulsePanels'
import { PulseRing } from '@/features/knowledge-pulse/PulseRing'
import {
  EVENT_KIND_LABEL,
  RESOURCE_TYPE_COLOR,
  RESOURCE_TYPE_LABEL,
} from '@/features/knowledge-pulse/pulseTheme'
import type {
  PulseEventKind,
  PulseResourceRef,
  PulseResourceType,
} from '@/features/knowledge-pulse/types'
import {
  POLL_INTERVALS_MS,
  type PollInterval,
  useKnowledgePulse,
} from '@/features/knowledge-pulse/useKnowledgePulse'
import { buildResourceUrl } from '@/lib/resourceUrl'
import { cn } from '@/lib/utils'

const INTERVAL_OPTIONS: readonly SegmentedOption[] = POLL_INTERVALS_MS.map(
  ms => ({ value: String(ms), label: `${String(ms / 1000)}s` })
)

const LEGEND_KINDS: readonly PulseEventKind[] = ['read', 'write', 'link']
const LEGEND_TYPES: readonly PulseResourceType[] = [
  'memory',
  'artifact',
  'prompt',
  'blueprint',
]

function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () =>
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => {
      setReduced(mq.matches)
    }
    mq.addEventListener('change', onChange)
    return () => {
      mq.removeEventListener('change', onChange)
    }
  }, [])
  return reduced
}

function LiveStatus({
  playing,
  hidden,
  error,
  intervalMs,
}: Readonly<{
  playing: boolean
  hidden: boolean
  error: boolean
  intervalMs: number
}>) {
  let label = 'Paused'
  let tone = 'bg-muted-foreground'
  if (error) {
    label = 'Connection problem, retrying'
    tone = 'bg-destructive'
  } else if (playing && hidden) {
    label = 'Paused while the tab is hidden'
  } else if (playing) {
    label = `Live · every ${String(intervalMs / 1000)}s`
    tone = 'bg-success'
  }
  const live = playing && !hidden && !error
  return (
    <span
      className="text-muted-foreground inline-flex items-center gap-2 text-xs font-medium"
      role="status"
    >
      <span className="relative flex size-2.5">
        {live && (
          <span
            className={cn(
              'absolute inline-flex size-full animate-ping rounded-full opacity-60',
              tone
            )}
          />
        )}
        <span
          className={cn('relative inline-flex size-2.5 rounded-full', tone)}
        />
      </span>
      {label}
    </span>
  )
}

/**
 * Knowledge Pulse: a live view of the team's shared brain being read,
 * written and linked by people and agents. `?autoplay=1` starts it playing,
 * and the full-screen button turns it into a TV view with no app chrome.
 */
export function KnowledgePulse() {
  const { currentTeam } = useTeam()
  const teamId = currentTeam?.id
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const pulse = useKnowledgePulse(teamId)
  const { state, playing, setPlaying, intervalMs, setIntervalMs } = pulse
  const reducedMotion = usePrefersReducedMotion()
  const stageRef = useRef<HTMLDivElement>(null)
  const [fullscreen, setFullscreen] = useState(false)

  useEffect(() => {
    if (params.get('autoplay') === '1') setPlaying(true)
  }, [params, setPlaying])

  // Space toggles play/pause, unless the user is typing or on a control.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== 'Space') return
      const t = e.target as HTMLElement | null
      if (
        t &&
        t !== document.body &&
        t.closest(
          'input,textarea,button,[role=button],select,[contenteditable]'
        )
      )
        return
      e.preventDefault()
      setPlaying(!playing)
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [playing, setPlaying])

  useEffect(() => {
    const onChange = () => {
      setFullscreen(document.fullscreenElement === stageRef.current)
    }
    document.addEventListener('fullscreenchange', onChange)
    return () => {
      document.removeEventListener('fullscreenchange', onChange)
    }
  }, [])

  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) {
      void document.exitFullscreen()
    } else {
      void stageRef.current?.requestFullscreen()
    }
  }, [])

  const recent = useMemo(
    () => new Set(state.feed.slice(0, 40).map(e => e.resource.id)),
    [state.feed]
  )
  const ringNodes = useMemo(
    () => capNodes([...state.nodes.values()], recent),
    [state.nodes, recent]
  )
  const actors = useMemo(() => [...state.actors.values()], [state.actors])

  const openResource = useCallback(
    (ref: Pick<PulseResourceRef, 'type' | 'id' | 'slug' | 'project_id'>) => {
      const url =
        ref.type === 'feed_item'
          ? `/feeds/items/${encodeURIComponent(ref.id)}`
          : buildResourceUrl({
              type: ref.type,
              id: ref.id,
              slug: ref.slug,
              projectId: ref.project_id,
            })
      if (url) void navigate(url)
    },
    [navigate]
  )

  const hiddenCount = state.nodes.size - ringNodes.length
  // While live, every counter climbs across the whole gap between polls, so
  // a jump from 500 to 700 reads as steady growth rather than a snap.
  const tweenMs = playing ? Math.round(intervalMs * 0.9) : 1200

  const controls = (
    <div className="flex flex-wrap items-center gap-3">
      <LiveStatus
        playing={playing}
        hidden={pulse.hidden}
        error={pulse.error !== null && !pulse.loading}
        intervalMs={intervalMs}
      />
      <SegmentedControl
        size="sm"
        options={INTERVAL_OPTIONS}
        value={String(intervalMs)}
        onChange={v => {
          setIntervalMs(Number(v) as PollInterval)
        }}
        aria-label="Refresh interval"
      />
      <Button
        size="sm"
        onClick={() => {
          setPlaying(!playing)
        }}
        disabled={pulse.loading || !teamId}
        aria-pressed={playing}
        className="min-w-24"
      >
        {playing ? (
          <>
            <Pause aria-hidden="true" /> Pause
          </>
        ) : (
          <>
            <Play aria-hidden="true" /> Play
          </>
        )}
      </Button>
      <Button
        size="icon-sm"
        variant="outline"
        onClick={toggleFullscreen}
        aria-label={fullscreen ? 'Exit full screen' : 'Full screen'}
        title={fullscreen ? 'Exit full screen' : 'Full screen (TV mode)'}
      >
        {fullscreen ? (
          <Minimize2 aria-hidden="true" />
        ) : (
          <Maximize2 aria-hidden="true" />
        )}
      </Button>
    </div>
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title="Knowledge Pulse"
        description="Watch your team's people and agents read, write and link the shared brain as it happens."
        actions={controls}
        className="pb-0"
      />

      {pulse.error && !pulse.loading && state.nodes.size === 0 && (
        <Alert variant="destructive">
          <AlertDescription>
            Couldn&apos;t load the knowledge pulse: {pulse.error}
          </AlertDescription>
        </Alert>
      )}

      <div
        ref={stageRef}
        className={cn(
          'grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]',
          fullscreen && 'bg-background h-screen overflow-hidden p-6'
        )}
      >
        <section
          aria-label="Knowledge ring"
          className="relative flex bg-[radial-gradient(circle_at_50%_55%,color-mix(in_oklch,var(--muted)_90%,transparent)_0%,transparent_65%)] min-h-0 flex-col rounded-xl border p-3 sm:p-4"
        >
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs">
            {LEGEND_KINDS.map(kind => (
              <span
                key={kind}
                className="text-muted-foreground inline-flex items-center gap-1.5"
              >
                <KindDot kind={kind} className="size-2.5" />
                {EVENT_KIND_LABEL[kind]}
              </span>
            ))}
            <span className="bg-border mx-1 hidden h-4 w-px sm:inline-block" />
            {LEGEND_TYPES.map(type => (
              <span
                key={type}
                className="text-muted-foreground inline-flex items-center gap-1.5"
              >
                <span
                  aria-hidden="true"
                  className="inline-block size-2.5 rounded-sm"
                  style={{ backgroundColor: RESOURCE_TYPE_COLOR[type] }}
                />
                {RESOURCE_TYPE_LABEL[type]}
              </span>
            ))}
            {fullscreen && <span className="ml-auto">{controls}</span>}
          </div>

          {pulse.loading ? (
            <Skeleton className="mx-auto mt-4 aspect-square w-full max-w-[min(100%,46rem)] rounded-full" />
          ) : (
            <PulseRing
              nodes={ringNodes}
              edges={state.edges}
              actors={actors}
              pulses={state.pulses}
              lastActivity={state.lastActivity}
              reducedMotion={reducedMotion}
              live={playing && !pulse.hidden}
              tweenMs={tweenMs}
              onOpen={openResource}
              className={cn(
                'mx-auto mt-2 w-full',
                fullscreen
                  ? 'max-w-[min(100%,calc(100vh-8rem))]'
                  : 'max-w-[46rem]'
              )}
            />
          )}

          {!playing && !pulse.loading && state.nodes.size > 0 && (
            <button
              type="button"
              onClick={() => {
                setPlaying(true)
              }}
              className="bg-background/80 hover:bg-background absolute bottom-4 left-1/2 inline-flex -translate-x-1/2 items-center gap-2 rounded-full border px-4 py-2 text-sm font-medium shadow-sm backdrop-blur transition-colors"
            >
              <Play className="size-4" aria-hidden="true" />
              Press play to watch it live
              <kbd className="text-muted-foreground ml-1 rounded border px-1.5 text-[10px]">
                Space
              </kbd>
            </button>
          )}

          {hiddenCount > 0 && (
            <p className="text-muted-foreground absolute right-4 bottom-4 text-xs">
              Showing the {ringNodes.length} most connected and active of{' '}
              {state.nodes.size}
            </p>
          )}
        </section>

        <EventFeed
          events={state.feed}
          onOpen={openResource}
          className={cn(
            'max-h-[28rem] xl:max-h-[calc(46rem+5rem)]',
            fullscreen && 'xl:max-h-none'
          )}
        />
      </div>

      {!fullscreen && (
        <>
          <StatTiles
            stats={state.stats}
            history={state.history}
            tweenMs={tweenMs}
            reducedMotion={reducedMotion}
          />
          <TopItems
            nodes={state.nodes}
            touches={state.touches}
            onOpen={openResource}
          />
        </>
      )}
    </div>
  )
}
