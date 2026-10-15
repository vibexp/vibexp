import { type ReactNode, useState } from 'react'

import { PLUGIN_COMMANDS, type PluginMode } from '../content'
import { Code, CommandRow, Lead, Segmented, StepIntro } from '../parts'

const MODES = [
  { value: 'claude', label: 'Inside Claude Code' },
  { value: 'term', label: 'From your terminal' },
] as const

function NumberedItem({
  n,
  children,
}: Readonly<{ n: number; children: ReactNode }>) {
  return (
    <li className="flex gap-4">
      <span
        aria-hidden="true"
        className="grid size-[26px] flex-none place-items-center rounded-full border text-xs font-semibold"
      >
        {n}
      </span>
      {children}
    </li>
  )
}

export function PluginStep() {
  const [mode, setMode] = useState<PluginMode>('claude')
  const cmd = PLUGIN_COMMANDS[mode]

  return (
    <>
      <StepIntro
        eyebrow="Connect Claude Code"
        title="Install the VibeXP plugin"
      >
        <Lead>
          The plugin adds the VibeXP commands to Claude Code:{' '}
          <Code>/vibexp:prime</Code>, <Code>/vibexp:wrap</Code> and{' '}
          <Code>/vibexp:install</Code>.
        </Lead>
      </StepIntro>

      <div className="self-start">
        <Segmented
          label="Where to run the commands"
          options={MODES}
          value={mode}
          onChange={setMode}
        />
      </div>

      <ol className="m-0 flex list-none flex-col gap-5 p-0">
        <NumberedItem n={1}>
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <span className="pt-[3px] text-[15px] font-medium">
              Add the VibeXP marketplace
            </span>
            <CommandRow command={cmd.add} label="marketplace command" />
          </div>
        </NumberedItem>
        <NumberedItem n={2}>
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <span className="pt-[3px] text-[15px] font-medium">
              Install the plugin
            </span>
            <CommandRow command={cmd.install} label="install command" />
          </div>
        </NumberedItem>
        <NumberedItem n={3}>
          <div className="flex flex-1 flex-col gap-1 pt-[3px]">
            <span className="text-[15px] font-medium">Restart Claude Code</span>
            <span className="text-muted-foreground text-sm">
              The VibeXP commands show up when you type{' '}
              <Code size={13}>/vibexp</Code>.
            </span>
          </div>
        </NumberedItem>
      </ol>
    </>
  )
}
