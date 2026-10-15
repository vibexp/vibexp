import { Icon } from '@vibexp/design-system/react'
import {
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { Helmet } from 'react-helmet-async'
import { useNavigate, useSearchParams } from 'react-router'

import { useAuth } from '@/contexts/useAuth'
import { siteTitleSuffix } from '@/features/site-info/siteInfo'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import { rememberAutoOpened, STEP_PARAM } from './autoOpen'
import { LAST_STEP, NEXT_LABELS, ONBOARDING_STEPS, STEP_COUNT } from './content'
import { OnboardingBrand } from './OnboardingBrand'
import {
  clampStep,
  resumeStep,
  type StepStatus,
  useOnboardingProgress,
  withStepStatus,
} from './progress'
import { STEP_HEADING_ATTR } from './shared'
import { CliStep } from './steps/CliStep'
import { ConceptsStep } from './steps/ConceptsStep'
import { DoneStep } from './steps/DoneStep'
import { PluginStep } from './steps/PluginStep'
import { RulesStep } from './steps/RulesStep'
import { WelcomeStep } from './steps/WelcomeStep'

function StepRail({
  step,
  status,
  onGo,
}: Readonly<{
  step: number
  status: readonly StepStatus[]
  onGo: (step: number) => void
}>) {
  return (
    // The design's `rail` navigation; below lg it collapses to its
    // `progress` mode: the header's count and bar alone.
    <nav
      aria-label="Onboarding steps"
      className="sticky top-24 hidden min-w-[200px] flex-[0_1_240px] lg:block"
    >
      <ol className="m-0 flex list-none flex-col gap-0.5 p-0">
        {ONBOARDING_STEPS.map((s, i) => {
          const active = i === step
          const st = status[i]
          const done = st === 'done' && !active
          let dot = 'border-border bg-transparent text-muted-foreground'
          if (done) dot = 'border-transparent bg-primary text-background'
          else if (active)
            dot = 'border-transparent bg-foreground text-background'
          else if (st === 'skipped')
            dot = 'border-muted-foreground bg-transparent text-muted-foreground'
          let state = ''
          if (st === 'done') state = ', done'
          else if (st === 'skipped') state = ', skipped'
          return (
            <li key={s.title}>
              <button
                type="button"
                aria-current={active ? 'step' : undefined}
                onClick={() => {
                  onGo(i)
                }}
                className={cn(
                  'text-foreground hover:bg-accent flex w-full cursor-pointer items-start gap-3 rounded-md border-none p-2.5 text-left',
                  active ? 'bg-accent' : 'bg-transparent'
                )}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'grid size-6 flex-none place-items-center rounded-full border text-xs font-semibold',
                    dot
                  )}
                >
                  {done ? <Icon name="check" size={13} stroke={3} /> : i + 1}
                </span>
                <span className="flex flex-col gap-0.5 pt-0.5">
                  <span
                    className={cn(
                      'text-sm',
                      active ? 'font-semibold' : 'font-medium',
                      active || done
                        ? 'text-foreground'
                        : 'text-muted-foreground'
                    )}
                  >
                    {s.title}
                    <span className="sr-only">{state}</span>
                  </span>
                  <span className="text-muted-foreground text-xs">
                    {s.description}
                  </span>
                </span>
              </button>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

function FooterButton({
  variant,
  onClick,
  disabled,
  children,
}: Readonly<{
  variant: 'primary' | 'outline' | 'quiet'
  onClick: () => void
  disabled?: boolean
  children: ReactNode
}>) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        ' inline-flex h-10 cursor-pointer items-center rounded-md text-sm disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'primary' &&
          // `ml-auto` keeps it on the right when it wraps onto its own row.
          'bg-primary text-primary-foreground ml-auto gap-2 border-none px-[18px] font-medium enabled:hover:opacity-[0.88]',
        variant === 'outline' &&
          'bg-background text-foreground hover:bg-accent gap-1.5 border px-3.5 font-medium',
        variant === 'quiet' &&
          'text-muted-foreground hover:text-foreground border-none bg-transparent px-3.5'
      )}
    >
      {children}
    </button>
  )
}

/**
 * The first-run onboarding (`/getting-started`): six steps from what VibeXP
 * is to a working Claude Code setup, every one skippable, replayable from the
 * profile menu's "Getting started". Full-screen with its own header, outside the
 * app shell, as designed.
 *
 * `?step=N` is the step on screen; without it the flow resumes at the first
 * step not done yet. Per-step progress lives in this browser
 * (`useOnboardingProgress`); finishing or skipping the whole flow sets the
 * user's `onboarding_completed`, so it no longer opens on its own.
 */
export function GettingStartedPage() {
  const { user, markOnboardingComplete } = useAuth()
  const { company_name } = useSiteInfo()
  const [progress, updateProgress] = useOnboardingProgress(user?.id)
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const [leaving, setLeaving] = useState(false)
  const contentRef = useRef<HTMLElement>(null)

  const rawStep = params.get(STEP_PARAM)
  const step =
    rawStep === null ? resumeStep(progress) : clampStep(Number(rawStep))

  // Pin the step in the URL, so marking it done does not move the screen and
  // a reload or a shared link opens the same step.
  useEffect(() => {
    if (rawStep === null || String(step) !== rawStep) {
      setParams({ [STEP_PARAM]: String(step) }, { replace: true })
    }
  }, [rawStep, step, setParams])

  useEffect(() => {
    updateProgress(p => (p.lastStep === step ? p : { ...p, lastStep: step }))
  }, [step, updateProgress])

  // A new step starts at the top of the page, and its heading takes focus,
  // so a keyboard or screen-reader user starts reading it from there. Not on
  // first load: the page is new then. A layout effect, so the old offset is
  // never painted (and the browser's scroll anchoring has nothing to keep).
  const firstRender = useRef(true)
  useLayoutEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    window.scrollTo({ top: 0, behavior: 'instant' })
    contentRef.current
      ?.querySelector<HTMLElement>(`[${STEP_HEADING_ATTR}]`)
      ?.focus({ preventScroll: true })
  }, [step])

  const go = (target: number) => {
    setParams({ [STEP_PARAM]: String(clampStep(target)) })
  }

  /** Leave the flow for good: Skip setup, or Go to VibeXP at the end. */
  const finish = async () => {
    if (!user) return
    setLeaving(true)
    // However it ends, the flow does not open on its own again this session.
    rememberAutoOpened(user.id)
    if (!user.onboarding_completed) {
      try {
        await markOnboardingComplete()
      } catch {
        toast.error('Could not save that you finished setting up', {
          description: 'Getting started may open again next time you sign in.',
        })
      }
    }
    void navigate('/')
  }

  const next = () => {
    if (step === LAST_STEP) {
      void finish()
      return
    }
    updateProgress(p => withStepStatus(p, step, 'done'))
    go(step + 1)
  }

  const skip = () => {
    updateProgress(p => withStepStatus(p, step, 'skipped'))
    go(step + 1)
  }

  const progressPct = Math.round((step / LAST_STEP) * 100)

  return (
    <div className="bg-background text-foreground min-h-screen antialiased">
      <Helmet>
        <title>{`Getting started · ${siteTitleSuffix(company_name)}`}</title>
      </Helmet>
      <header className="bg-background sticky top-0 z-[2]">
        <div className="flex items-center gap-3 px-4 py-3.5 sm:gap-4 sm:px-6">
          <OnboardingBrand />
          <span
            aria-hidden="true"
            className="bg-border hidden h-[18px] w-px sm:block"
          />
          <span className="text-muted-foreground hidden text-sm sm:inline">
            Getting started
          </span>
          <div className="flex-1" />
          <span className="text-muted-foreground text-[13px] whitespace-nowrap tabular-nums">
            <span className="sr-only">Step </span>
            {step + 1}
            <span aria-hidden="true"> / </span>
            <span className="sr-only"> of </span>
            {STEP_COUNT}
          </span>
          <button
            type="button"
            onClick={() => {
              void finish()
            }}
            disabled={leaving}
            className="text-muted-foreground hover:bg-accent hover:text-foreground inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border-none bg-transparent px-2.5 text-sm whitespace-nowrap disabled:opacity-50"
          >
            Skip setup
            <Icon name="x" size={15} />
          </button>
        </div>
        <div
          role="progressbar"
          aria-label="Onboarding progress"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={progressPct}
          className="bg-border h-0.5"
        >
          <div
            className="bg-primary h-full transition-[width] duration-[350ms] ease-in-out"
            style={{ width: `${String(progressPct)}%` }}
          />
        </div>
      </header>

      <div className="mx-auto flex max-w-[1160px] flex-wrap items-start gap-12 px-4 pt-8 pb-16 sm:px-6 sm:pt-12">
        <StepRail step={step} status={progress.status} onGo={go} />

        <main
          ref={contentRef}
          aria-label={ONBOARDING_STEPS[step].title}
          // Centred while the rail is hidden; beside it from lg.
          className="mx-auto flex max-w-[760px] min-w-0 flex-[999_1_520px] flex-col gap-8 lg:mx-0"
        >
          {step === 0 && <WelcomeStep />}
          {step === 1 && <ConceptsStep />}
          {step === 2 && <PluginStep />}
          {step === 3 && <RulesStep />}
          {step === 4 && <CliStep />}
          {step === 5 && <DoneStep status={progress.status} onGoToStep={go} />}

          <div className="flex flex-wrap items-center gap-2 border-t pt-6">
            {step > 0 && (
              <FooterButton
                variant="outline"
                onClick={() => {
                  go(step - 1)
                }}
              >
                <Icon name="chevronLeft" size={15} />
                Back
              </FooterButton>
            )}
            {step === 0 && (
              <span className="text-muted-foreground inline-flex items-center gap-1.5 text-[13px]">
                <Icon name="clock" size={14} />
                About 5 minutes
              </span>
            )}
            <div className="flex-1" />
            {step > 0 && step < LAST_STEP && (
              <FooterButton variant="quiet" onClick={skip}>
                Skip this step
              </FooterButton>
            )}
            <FooterButton
              variant="primary"
              onClick={next}
              disabled={step === LAST_STEP && leaving}
            >
              {NEXT_LABELS[step]}
              <Icon name="arrowRight" size={15} />
            </FooterButton>
          </div>
        </main>
      </div>
    </div>
  )
}
