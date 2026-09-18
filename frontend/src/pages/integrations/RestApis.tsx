import { Braces, Check, Copy, ExternalLink } from 'lucide-react'

import { Card } from '@/components/ui/card'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

// Served publicly (no auth) by the backend's setupPublicRoutes() on the same
// origin as the app. Built from window.location.origin at render time, never
// a build-time value, so the links are real on any self-hosted instance.
const SPEC_FORMATS = [
  { id: 'yaml', label: 'YAML', path: '/openapi.yaml' },
  { id: 'json', label: 'JSON', path: '/openapi.json' },
]

const USES = [
  {
    id: 'clients',
    text: 'Generate a typed API client in any language with an OpenAPI code generator.',
  },
  {
    id: 'tools',
    text: 'Import it into Postman, Insomnia or editor.swagger.io to explore and call the endpoints.',
  },
  {
    id: 'agents',
    text: 'Give it to an AI agent or script as the exact contract for this instance.',
  },
]

function SpecUrlRow({ label, url }: Readonly<{ label: string; url: string }>) {
  const { copied, copy } = useCopyToClipboard()

  return (
    <div className="flex flex-wrap items-center gap-3.5">
      <div className="border-input bg-muted flex min-w-[280px] flex-1 items-center gap-3 rounded-md border py-3 pl-4 pr-2">
        <span className="text-muted-foreground bg-background rounded-full border px-[7px] py-[3px] text-xs font-bold uppercase tracking-wider">
          {label}
        </span>
        <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-base font-medium">
          {url}
        </span>
      </div>
      <a
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={`Open ${label} schema`}
        className="bg-secondary text-secondary-foreground hover:bg-secondary/80 inline-flex h-11 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors"
      >
        <ExternalLink className="size-4" />
        Open
      </a>
      <button
        type="button"
        onClick={() => {
          copy(url)
        }}
        aria-label={`Copy ${label} schema URL`}
        className="bg-secondary text-secondary-foreground hover:bg-secondary/80 inline-flex h-11 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors"
      >
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  )
}

export function RestApis() {
  const origin = window.location.origin

  return (
    <div className="mx-auto max-w-[1080px]">
      <header className="mb-8">
        <h1 className="flex items-center gap-3 text-3xl font-bold tracking-tight">
          <span className="bg-primary text-primary-foreground grid size-[38px] place-items-center rounded-md">
            <Braces className="size-[21px]" />
          </span>
          {'REST APIs'}
        </h1>
        <p className="text-muted-foreground mt-2.5 text-base">
          This instance publishes its full REST API contract as an OpenAPI
          schema — no sign-in needed to fetch it.
        </p>
      </header>

      <section>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">
          OpenAPI schema
        </h2>
        <div className="space-y-3">
          {SPEC_FORMATS.map(format => (
            <SpecUrlRow
              key={format.id}
              label={format.label}
              url={`${origin}${format.path}`}
            />
          ))}
        </div>
      </section>

      <section className="mb-20 mt-9">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">
          What you can do with it
        </h2>
        <Card className="p-[22px]">
          <ul className="text-muted-foreground list-disc space-y-2 pl-5 text-sm">
            {USES.map(use => (
              <li key={use.id}>{use.text}</li>
            ))}
          </ul>
        </Card>
      </section>
    </div>
  )
}
