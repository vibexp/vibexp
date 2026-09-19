import { ExternalLink, Terminal } from 'lucide-react'

import { Card } from '@/components/ui/card'
import { CodeBlock } from '@/pages/mcp/CodeBlock'

const CLI_REPO_URL = 'https://github.com/vibexp/cli'
const CLI_RELEASES_URL = `${CLI_REPO_URL}/releases/latest`

// Mirrors the install table in the vibexp/cli README - keep the two in sync.
const INSTALL_METHODS = [
  {
    id: 'homebrew',
    label: 'Homebrew (macOS)',
    code: 'brew install vibexp/tap/vibexp',
  },
  {
    id: 'go-install',
    label: 'go install',
    code: 'go install github.com/vibexp/cli/cmd/vibexp@latest',
  },
]

const CAPABILITIES = [
  {
    id: 'auth',
    text: 'Sign in with an OAuth 2.1 browser login, or an API key for CI and scripts.',
  },
  {
    id: 'contexts',
    text: 'Switch between deployments and teams with kubectl-style contexts.',
  },
  {
    id: 'resources',
    text: 'Manage memories, blueprints, prompts, artifacts, feeds, attachments and relations, and run semantic search.',
  },
  {
    id: 'api',
    text: (
      <>
        Call any REST endpoint directly with the{' '}
        <code className="font-mono">vibexp api</code> escape hatch.
      </>
    ),
  },
]

const copyToClipboard = (code: string) => {
  void navigator.clipboard.writeText(code)
}

export function CliPage() {
  return (
    <div className="mx-auto max-w-[1080px]">
      <header className="mb-8">
        <h1 className="flex items-center gap-3 text-3xl font-bold tracking-tight">
          <span className="bg-primary text-primary-foreground grid size-[38px] place-items-center rounded-md">
            <Terminal className="size-[21px]" />
          </span>
          {'VibeXP CLI'}
        </h1>
        <p className="text-muted-foreground mt-2.5 text-base">
          Work with your team&apos;s knowledge from the terminal — and from any
          script or agent that can run a shell command.
        </p>
      </header>

      <section>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Install</h2>
        <div className="space-y-3">
          {INSTALL_METHODS.map(method => (
            <CodeBlock
              key={method.id}
              code={method.code}
              language="shell"
              file={method.label}
              onCopy={copyToClipboard}
            />
          ))}
        </div>
        <p className="text-muted-foreground mt-3 text-sm">
          Or download a prebuilt binary for Linux, macOS or Windows from the{' '}
          <a
            href={CLI_RELEASES_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground font-medium underline underline-offset-4"
          >
            latest release
          </a>
          {'.'}
        </p>
      </section>

      <section className="mt-9">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">
          What you can do
        </h2>
        <Card className="p-[22px]">
          <ul className="text-muted-foreground list-disc space-y-2 pl-5 text-sm">
            {CAPABILITIES.map(capability => (
              <li key={capability.id}>{capability.text}</li>
            ))}
          </ul>
        </Card>
      </section>

      <section className="mb-20 mt-9">
        <a
          href={CLI_REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="bg-secondary text-secondary-foreground hover:bg-secondary/80 inline-flex h-11 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors"
        >
          <ExternalLink className="size-4" />
          Full documentation on GitHub
        </a>
      </section>
    </div>
  )
}
