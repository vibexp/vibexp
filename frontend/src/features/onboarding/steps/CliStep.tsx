import { Icon } from '@vibexp/design-system/react'

import { cliInstallCommand } from '../content'
import { Code, CopyIcon, Lead, StepIntro } from '../parts'
import { EYEBROW, useCopy } from '../shared'

const BENEFITS = [
  'Installs the VibeXP CLI on your machine',
  'Prepares it for your VibeXP account and teams',
] as const

export function CliStep() {
  const { copied, copy } = useCopy()
  const command = cliInstallCommand(window.location.origin)
  const isCopied = copied === 'cli'

  return (
    <>
      <StepIntro
        eyebrow={
          <span className="flex items-center gap-2.5">
            <span className={EYEBROW}>VibeXP CLI</span>
            <span className="bg-primary text-primary-foreground rounded-full px-2 py-0.5 text-xs font-medium">
              Recommended
            </span>
          </span>
        }
        title="Let Claude set up the CLI"
      >
        <Lead className="max-w-[640px]">
          With the CLI, your agents talk to VibeXP through short shell commands
          instead of tool calls. That keeps priming and wrapping light on
          tokens. Open a terminal and run:
        </Lead>
      </StepIntro>

      <div className="bg-primary text-primary-foreground flex items-center gap-3 rounded-lg py-2.5 pr-2.5 pl-[18px]">
        <span aria-hidden="true" className="font-mono text-[15px] opacity-60">
          $
        </span>
        <code className="min-w-0 flex-1 overflow-x-auto font-mono text-[15px] leading-9 whitespace-pre">
          {command}
        </code>
        <button
          type="button"
          onClick={() => {
            copy('cli', command)
          }}
          aria-label={
            isCopied ? 'Copied install command' : 'Copy install command'
          }
          className="bg-primary-foreground text-primary inline-flex h-8 flex-none cursor-pointer items-center gap-1.5 rounded-sm px-3 text-[13px] font-medium"
        >
          <CopyIcon copied={isCopied} />
          <span aria-hidden="true">{isCopied ? 'Copied' : 'Copy'}</span>
        </button>
      </div>

      <ul className="m-0 flex list-none flex-col gap-2.5 p-0 text-sm leading-[1.5]">
        {BENEFITS.map(benefit => (
          <li key={benefit} className="flex items-center gap-2.5">
            <Icon name="check" size={15} />
            {benefit}
          </li>
        ))}
      </ul>

      <div className="bg-card flex flex-col gap-2.5 rounded-lg border p-5">
        <span className="text-[15px] font-semibold">
          Then try it in any project
        </span>
        <span className="text-muted-foreground text-sm leading-[1.55]">
          Open Claude in a repo and run <Code size={13}>/vibexp:prime</Code> to
          load what your team knows. When you&apos;re done,{' '}
          <Code size={13}>/vibexp:wrap</Code> saves what&apos;s worth keeping.
        </span>
      </div>
    </>
  )
}
