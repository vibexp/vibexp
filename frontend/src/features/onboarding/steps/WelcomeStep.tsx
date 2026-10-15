import { WELCOME_VIDEO_ID } from '../content'
import { EYEBROW, STEP_HEADING_ATTR } from '../shared'

const LOOP = [
  {
    n: '01',
    title: 'Read',
    text: "Load the team's knowledge before work starts.",
    code: '/vibexp:prime',
    muted: false,
  },
  {
    n: '02',
    title: 'Learn',
    text: 'Capture facts, fixes and guidelines as you go.',
    code: 'during the session',
    muted: true,
  },
  {
    n: '03',
    title: 'Write',
    text: "Save what's shareable so the next session is smarter.",
    code: '/vibexp:wrap',
    muted: false,
  },
] as const

export function WelcomeStep() {
  return (
    <>
      <div className="flex flex-col gap-3.5">
        <span className={EYEBROW}>Welcome to VibeXP</span>
        <h1
          {...{ [STEP_HEADING_ATTR]: '' }}
          tabIndex={-1}
          className="onboarding-step-heading m-0 text-[length:var(--text-fluid-title)] leading-[1.08] font-bold tracking-[-0.03em] text-balance"
        >
          Your team&apos;s shared brain for AI agents
        </h1>
        <p className="text-muted-foreground m-0 max-w-[640px] text-[length:var(--text-fluid-lead)] leading-[1.6] text-pretty">
          Every Claude session starts from zero. VibeXP gives your agents one
          shared memory across the team: they read what the team already knows,
          learn while they work, and write back what&apos;s worth keeping.
        </p>
      </div>

      <div className="bg-muted relative aspect-video overflow-hidden rounded-xl border">
        <iframe
          src={`https://www.youtube-nocookie.com/embed/${WELCOME_VIDEO_ID}?rel=0`}
          title="How VibeXP's shared brain works"
          className="absolute inset-0 size-full border-0"
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowFullScreen
          loading="lazy"
          referrerPolicy="strict-origin-when-cross-origin"
        />
      </div>

      <ol className="bg-border m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(190px,1fr))] gap-px overflow-hidden rounded-lg border p-0">
        {LOOP.map(item => (
          <li key={item.n} className="bg-card flex flex-col gap-1.5 p-[18px]">
            <span className="text-muted-foreground text-xs tabular-nums">
              {item.n}
            </span>
            <span className="text-base font-semibold">{item.title}</span>
            <span className="text-muted-foreground text-sm leading-[1.5]">
              {item.text}
            </span>
            <code
              className={
                item.muted
                  ? 'text-muted-foreground mt-1 font-mono text-xs'
                  : 'mt-1 font-mono text-xs'
              }
            >
              {item.code}
            </code>
          </li>
        ))}
      </ol>
    </>
  )
}
