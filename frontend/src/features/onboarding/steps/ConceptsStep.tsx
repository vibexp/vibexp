import * as TabsPrimitive from '@radix-ui/react-tabs'
import { Icon, type IconName } from '@vibexp/design-system/react'
import { type ReactNode, useState } from 'react'

import { Lead, StepIntro } from '../parts'

const EXAMPLE_CARD = 'bg-background flex flex-col rounded-lg border'

function MemoryExample() {
  return (
    <div className={`${EXAMPLE_CARD} gap-2 p-4`}>
      <span className="text-muted-foreground text-xs">
        Memory · payments-api
      </span>
      <span className="text-sm leading-[1.55]">
        Every POST to /charges must send an Idempotency-Key header. Retries
        without it caused duplicate charges in March.
      </span>
      <span className="text-muted-foreground text-xs">
        Saved by /vibexp:wrap from Nadia&apos;s session
      </span>
    </div>
  )
}

const BLUEPRINT_RULES = [
  'Return application/problem+json',
  'Include the request id in every error',
  'Never expose stack traces',
] as const

function BlueprintExample() {
  return (
    <div className={`${EXAMPLE_CARD} gap-2.5 p-4`}>
      <span className="text-muted-foreground text-xs">
        Blueprint · API error responses
      </span>
      <ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-sm">
        {BLUEPRINT_RULES.map(rule => (
          <li key={rule} className="flex items-center gap-2">
            <Icon name="check" size={14} />
            {rule}
          </li>
        ))}
      </ul>
    </div>
  )
}

function ArtifactExample() {
  return (
    <div className={`${EXAMPLE_CARD} flex-row items-center gap-3.5 p-4`}>
      <span className="bg-muted grid size-9 flex-none place-items-center rounded-md">
        <Icon name="fileText" size={17} />
      </span>
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="text-sm font-medium">
          Checkout latency investigation
        </span>
        <span className="text-muted-foreground text-xs">
          Report · checkout-web · 2 days ago
        </span>
      </span>
    </div>
  )
}

const FEED_ENTRIES = [
  {
    initials: 'NA',
    avatar: 'bg-avatar-2',
    who: "Nadia's agent",
    did: 'added a memory',
    what: 'Safari drops the session cookie on cross-site redirects.',
  },
  {
    initials: 'AR',
    avatar: 'bg-avatar-3',
    who: "Arif's agent",
    did: 'updated a blueprint',
    what: 'Terraform module naming',
  },
] as const

function FeedExample() {
  return (
    <ul className={`${EXAMPLE_CARD} m-0 list-none p-0`}>
      {FEED_ENTRIES.map((entry, i) => (
        <li
          key={entry.initials}
          className={`flex gap-3 px-4 py-3.5 ${i === 0 ? 'border-b' : ''}`}
        >
          <span
            aria-hidden="true"
            className={`${entry.avatar} text-primary-foreground grid size-7 flex-none place-items-center rounded-full text-[11px] font-semibold`}
          >
            {entry.initials}
          </span>
          <span className="flex flex-col gap-0.5 text-sm leading-[1.5]">
            <span>
              <strong className="font-semibold">{entry.who}</strong> {entry.did}
            </span>
            <span className="text-muted-foreground">{entry.what}</span>
          </span>
        </li>
      ))}
    </ul>
  )
}

const CONCEPTS: readonly {
  value: string
  label: string
  icon: IconName
  text: string
  example: ReactNode
}[] = [
  {
    value: 'memory',
    label: 'Memory',
    icon: 'book',
    text: 'Durable facts your agents should never have to rediscover: decisions, conventions, and how your systems fit together. Loaded every time a session primes.',
    example: <MemoryExample />,
  },
  {
    value: 'blueprints',
    label: 'Blueprints',
    icon: 'layout',
    text: 'Guidelines and rules written as reusable instructions, so every agent on the team does things the same way.',
    example: <BlueprintExample />,
  },
  {
    value: 'artifacts',
    label: 'Artifacts',
    icon: 'fileText',
    text: 'Reports and reference material produced during work: investigations, reviews, release notes and how-tos your team can read later.',
    example: <ArtifactExample />,
  },
  {
    value: 'feeds',
    label: 'AI Feeds',
    icon: 'inbox',
    text: 'A running stream of what agents across your team have learned, so people can review it, correct it, and stay in the loop.',
    example: <FeedExample />,
  },
]

export function ConceptsStep() {
  const [concept, setConcept] = useState(CONCEPTS[0].value)

  return (
    <>
      <StepIntro
        eyebrow="The building blocks"
        title="What lives in the shared brain"
      >
        <Lead>
          Four kinds of knowledge your agents read and write. Everything belongs
          to a team, and optionally to a project inside it.
        </Lead>
      </StepIntro>

      <TabsPrimitive.Root
        value={concept}
        onValueChange={setConcept}
        orientation="vertical"
        className="flex flex-wrap items-stretch gap-6"
      >
        <TabsPrimitive.List
          aria-label="Building blocks"
          className="flex flex-[1_1_200px] flex-col gap-1"
        >
          {CONCEPTS.map(c => (
            <TabsPrimitive.Trigger
              key={c.value}
              value={c.value}
              className="text-foreground data-[state=active]:bg-accent data-[state=active]:border-border flex cursor-pointer items-center gap-3 rounded-md border border-transparent px-3.5 py-3 text-left text-[15px] font-medium"
            >
              <Icon name={c.icon} size={17} />
              {c.label}
            </TabsPrimitive.Trigger>
          ))}
        </TabsPrimitive.List>

        {CONCEPTS.map(c => (
          <TabsPrimitive.Content
            key={c.value}
            value={c.value}
            className="bg-card flex min-w-0 flex-[999_1_340px] flex-col gap-[18px] rounded-xl border p-6 data-[state=inactive]:hidden"
          >
            <div className="flex flex-col gap-2">
              <h3 className="m-0 text-xl font-semibold tracking-[-0.018em]">
                {c.label}
              </h3>
              <p className="text-muted-foreground m-0 text-[15px] leading-[1.6] text-pretty">
                {c.text}
              </p>
            </div>
            {c.example}
          </TabsPrimitive.Content>
        ))}
      </TabsPrimitive.Root>
    </>
  )
}
