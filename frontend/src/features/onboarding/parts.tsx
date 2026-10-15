import { Icon } from '@vibexp/design-system/react'
import { type ReactNode, useRef } from 'react'

import { cn } from '@/lib/utils'

import { EYEBROW, STEP_HEADING_ATTR, useCopyCommand } from './shared'

/**
 * A step's overline, heading and lead. The welcome and celebration screens
 * use the larger title (`h1`); the steps between use the section size.
 */
export function StepIntro({
  eyebrow,
  title,
  level = 2,
  children,
}: Readonly<{
  eyebrow: ReactNode
  title: string
  level?: 1 | 2
  children: ReactNode
}>) {
  const Heading = level === 1 ? 'h1' : 'h2'
  return (
    <div className={cn('flex flex-col', level === 1 ? 'gap-3.5' : 'gap-3')}>
      {typeof eyebrow === 'string' ? (
        <span className={EYEBROW}>{eyebrow}</span>
      ) : (
        eyebrow
      )}
      <Heading
        {...{ [STEP_HEADING_ATTR]: '' }}
        tabIndex={-1}
        className={cn(
          'onboarding-step-heading m-0 font-bold',
          level === 1
            ? 'text-[length:var(--text-fluid-title)] leading-[1.08] tracking-[-0.03em] text-balance'
            : 'text-[length:var(--text-fluid-section)] leading-[1.1] tracking-[-0.018em]'
        )}
      >
        {title}
      </Heading>
      {children}
    </div>
  )
}

/** The lead paragraph under a section heading. */
export function Lead({
  children,
  className,
}: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <p
      className={cn(
        'text-muted-foreground m-0 max-w-[620px] text-base leading-[1.6] text-pretty',
        className
      )}
    >
      {children}
    </p>
  )
}

/** A command or path inside running text. */
export function Code({
  children,
  size = 14,
}: Readonly<{ children: ReactNode; size?: 13 | 14 }>) {
  return (
    <code
      className={cn(
        'text-foreground font-mono',
        size === 13 ? 'text-[13px]' : 'text-sm'
      )}
    >
      {children}
    </code>
  )
}

/** The copy button's icon: a tick for a moment after copying. */
export function CopyIcon({
  copied,
  size = 13,
}: Readonly<{ copied: boolean; size?: number }>) {
  return <Icon name={copied ? 'check' : 'copy'} size={size} />
}

/** A one-line command with its copy button (the plugin step). */
export function CommandRow({
  command,
  label,
}: Readonly<{
  command: string
  /** What is copied, for the button's accessible name. */
  label: string
}>) {
  const codeRef = useRef<HTMLElement>(null)
  const { copied, copy } = useCopyCommand(codeRef)
  return (
    <div className="bg-muted flex items-start gap-2 rounded-md border py-1.5 pr-1.5 pl-3.5 sm:items-center">
      {/* On a phone the whole command wraps rather than scrolling, so none
          of it is hidden. */}
      <code
        ref={codeRef}
        className="min-w-0 flex-1 overflow-x-auto font-mono text-[13px] leading-7 break-all whitespace-pre-wrap sm:break-normal sm:whitespace-pre"
      >
        {command}
      </code>
      <button
        type="button"
        onClick={() => {
          copy(command)
        }}
        aria-label={copied ? `Copied ${label}` : `Copy ${label}`}
        className="bg-background text-foreground hover:bg-accent inline-flex h-7 flex-none cursor-pointer items-center gap-1.5 rounded-sm border px-2.5 text-xs font-medium"
      >
        <CopyIcon copied={copied} />
        <span aria-hidden="true">{copied ? 'Copied' : 'Copy'}</span>
      </button>
    </div>
  )
}

/**
 * Two-option switch (the design's segmented control): plugin from inside
 * Claude Code or the terminal, rules pasted or appended.
 */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  height = 30,
}: Readonly<{
  /** `short` replaces the label below `sm`; the accessible name stays `label`. */
  options: readonly { value: T; label: string; short?: string }[]
  value: T
  onChange: (value: T) => void
  label: string
  height?: 28 | 30
}>) {
  return (
    // A fieldset rather than a div with role="group"; reset to look like
    // one. Its name is the legend, kept for screen readers only.
    <fieldset className="bg-muted m-0 inline-flex min-w-0 gap-0.5 rounded-md border-0 p-[3px]">
      <legend className="sr-only">{label}</legend>
      {options.map(option => {
        const on = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={on}
            aria-label={option.short ? option.label : undefined}
            onClick={() => {
              onChange(option.value)
            }}
            className={cn(
              'text-foreground cursor-pointer rounded-sm px-3 text-[13px] font-medium whitespace-nowrap',
              height === 28 ? 'h-7' : 'h-[30px]',
              on ? 'bg-background shadow-(--shadow-sm)' : 'bg-transparent'
            )}
          >
            {option.short ? (
              <>
                <span className="sm:hidden">{option.short}</span>
                <span className="hidden sm:inline">{option.label}</span>
              </>
            ) : (
              option.label
            )}
          </button>
        )
      })}
    </fieldset>
  )
}

/**
 * A thin progress bar: a native `<progress>`, so it has its role and value
 * without ARIA, drawn as the design's flat track and fill. The track colour
 * and height come from `className`; the fill is the primary colour. WebKit
 * and Blink style it through `::-webkit-progress-*` (the fill's width
 * animates), Firefox through `::-moz-progress-bar`.
 */
export function ThinProgress({
  value,
  label,
  className,
}: Readonly<{
  /** 0 to 100. */
  value: number
  label: string
  className?: string
}>) {
  return (
    <progress
      value={value}
      max={100}
      aria-label={label}
      className={cn(
        'block w-full appearance-none overflow-hidden border-0',
        '[&::-webkit-progress-bar]:bg-transparent',
        '[&::-webkit-progress-value]:bg-primary [&::-webkit-progress-value]:transition-[width] [&::-webkit-progress-value]:duration-300 [&::-webkit-progress-value]:ease-in-out',
        '[&::-moz-progress-bar]:bg-primary',
        className
      )}
    />
  )
}
