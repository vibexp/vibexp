import { Icon, LogoMark } from '@vibexp/design-system/react'

import { cn } from '@/lib/utils'

import { ONBOARDING_STEPS } from '../content'
import type { StepStatus } from '../progress'
import { STEP_HEADING_ATTR } from '../shared'

/** The steps the summary lists: the welcome screen has nothing to finish. */
const SUMMARY_STEPS = [1, 2, 3, 4] as const

const RING_DELAYS = ['0s', '0.8s', '1.6s'] as const

export function DoneStep({
  status,
  onGoToStep,
}: Readonly<{
  status: readonly StepStatus[]
  onGoToStep: (step: number) => void
}>) {
  return (
    <>
      <div className="flex flex-col items-start gap-5 pt-2">
        <div className="relative grid size-28 place-items-center">
          <div
            aria-hidden="true"
            className="absolute inset-0 motion-reduce:hidden"
          >
            {RING_DELAYS.map(delay => (
              <span
                key={delay}
                className="border-foreground absolute inset-0 animate-[vx-ring_2.4s_ease-out_infinite] rounded-full border-[1.5px] opacity-0"
                style={{ animationDelay: delay }}
              />
            ))}
          </div>
          <LogoMark size={64} aria-hidden="true" />
        </div>
        <h1
          {...{ [STEP_HEADING_ATTR]: '' }}
          tabIndex={-1}
          className="onboarding-step-heading m-0 text-[length:var(--text-fluid-title)] leading-[1.08] font-bold tracking-[-0.03em] text-balance"
        >
          Your agents now learn with your team
        </h1>
        <p className="text-muted-foreground m-0 max-w-[620px] text-[length:var(--text-fluid-lead)] leading-[1.6] text-pretty">
          Every session starts from what your team already knows and leaves
          something behind for the next one. Session one is good. Session one
          hundred knows your codebase, your rules and your history.
        </p>
      </div>

      <ul className="m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {SUMMARY_STEPS.map((step, n) => {
          const done = status[step] === 'done'
          const { title } = ONBOARDING_STEPS[step]
          return (
            <li
              key={step}
              className={cn(
                'flex items-center gap-3 px-4 py-3 text-sm',
                n > 0 && 'border-t'
              )}
            >
              <span
                className={cn(
                  'text-primary-foreground grid size-[22px] flex-none place-items-center rounded-full border',
                  done ? 'bg-primary border-transparent' : 'bg-transparent'
                )}
              >
                {done && <Icon name="check" size={12} stroke={3} />}
              </span>
              <span
                className={cn(
                  'flex-1',
                  done ? 'text-foreground' : 'text-muted-foreground'
                )}
              >
                {title}
                <span className="sr-only">
                  {done ? ', done' : ', not done'}
                </span>
              </span>
              {!done && (
                <button
                  type="button"
                  onClick={() => {
                    onGoToStep(step)
                  }}
                  aria-label={`Finish this step: ${title}`}
                  className="text-foreground cursor-pointer rounded-sm border-none bg-transparent p-0 text-[13px] font-medium underline underline-offset-[3px]"
                >
                  Finish this step
                </button>
              )}
            </li>
          )
        })}
      </ul>

      <p className="text-muted-foreground m-0 text-[13px]">
        You can replay this onboarding anytime from{' '}
        <strong className="text-foreground font-medium">Getting started</strong>{' '}
        in your profile menu.
      </p>
    </>
  )
}
