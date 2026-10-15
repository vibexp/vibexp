import { Icon } from '@vibexp/design-system/react'
import { useState } from 'react'

import { RULES, RULES_APPEND } from '../content'
import { Code, CopyIcon, Lead, Segmented, StepIntro } from '../parts'
import { useCopy } from '../shared'

type RulesMode = 'copy' | 'append'

const MODES = [
  { value: 'copy', label: 'Paste into the file', short: 'Paste' },
  { value: 'append', label: 'Append with one command', short: 'Append' },
] as const

export function RulesStep() {
  const [mode, setMode] = useState<RulesMode>('copy')
  const { copied, copy } = useCopy()
  const text = mode === 'append' ? RULES_APPEND : RULES
  const isCopied = copied === 'rules'
  let buttonLabel = mode === 'append' ? 'Copy command' : 'Copy rules'
  if (isCopied) buttonLabel = 'Copied'

  return (
    <>
      <StepIntro eyebrow="Team rules" title="Make it a habit in every session">
        <Lead className="max-w-[640px]">
          Add these rules to <Code>~/.claude/CLAUDE.md</Code>. Claude loads this
          file at the start of every session, so every project follows the read,
          learn, write loop without you asking.
        </Lead>
      </StepIntro>

      <div className="flex flex-col overflow-hidden rounded-lg border">
        <div className="bg-card flex flex-wrap items-center gap-2 border-b py-2 pr-2 pl-3">
          <Segmented
            label="How to add the rules"
            options={MODES}
            value={mode}
            onChange={setMode}
            height={28}
          />
          <div className="hidden flex-1 sm:block" />
          <button
            type="button"
            onClick={() => {
              copy('rules', text)
            }}
            className="bg-primary text-primary-foreground ml-auto inline-flex h-[30px] cursor-pointer items-center gap-1.5 rounded-sm px-3 text-[13px] font-medium hover:opacity-[0.88]"
          >
            <CopyIcon copied={isCopied} />
            {buttonLabel}
          </button>
        </div>
        <pre className="bg-muted text-foreground m-0 max-h-[340px] overflow-auto px-5 py-[18px] font-mono text-[13px] leading-[1.65] whitespace-pre-wrap">
          {text}
        </pre>
      </div>

      <div className="text-muted-foreground flex items-start gap-3 text-sm leading-[1.55]">
        <span className="text-foreground flex-none pt-0.5">
          <Icon name="shield" size={16} />
        </span>
        <span>
          Only team-relevant knowledge is shared. The rules tell Claude to keep
          personal information, secrets and customer data out of VibeXP.
        </span>
      </div>
    </>
  )
}
