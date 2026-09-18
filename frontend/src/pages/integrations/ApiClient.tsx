import { Boxes, ExternalLink } from 'lucide-react'

import { Card } from '@/components/ui/card'
import { CodeBlock } from '@/pages/mcp/CodeBlock'

// Both clients are generated from backend/openapi.yaml and auto-published on
// every spec change merged to main - full usage docs live in each repo.
const CLIENTS = [
  {
    id: 'go',
    title: 'Go',
    description:
      'A typed Go client generated with oapi-codegen. Requires Go 1.24 or later.',
    install: 'go get github.com/vibexp/api-client-go@latest',
    installLabel: 'go get',
    repoName: 'vibexp/api-client-go',
    repoUrl: 'https://github.com/vibexp/api-client-go',
  },
  {
    id: 'typescript',
    title: 'TypeScript / JavaScript',
    description:
      'A typed client for Node.js and the browser, built on openapi-fetch, with an optional axios-based SDK entrypoint.',
    install: 'npm install @vibexp/api-client',
    installLabel: 'npm',
    repoName: 'vibexp/api-client-js',
    repoUrl: 'https://github.com/vibexp/api-client-js',
  },
]

const copyToClipboard = (code: string) => {
  void navigator.clipboard.writeText(code)
}

export function ApiClient() {
  return (
    <div className="mx-auto mb-20 max-w-[1080px]">
      <header className="mb-8">
        <h1 className="flex items-center gap-3 text-3xl font-bold tracking-tight">
          <span className="bg-primary text-primary-foreground grid size-[38px] place-items-center rounded-md">
            <Boxes className="size-[21px]" />
          </span>
          {'API Client'}
        </h1>
        <p className="text-muted-foreground mt-2.5 text-base">
          Typed clients for the VibeXP REST API, generated from its OpenAPI
          schema — call any endpoint without hand-rolling HTTP requests or
          running your own code generator.
        </p>
      </header>

      <div className="space-y-9">
        {CLIENTS.map(client => (
          <section key={client.id}>
            <h2 className="mb-4 text-lg font-semibold tracking-tight">
              {client.title}
            </h2>
            <Card className="space-y-4 p-[22px]">
              <p className="text-muted-foreground text-sm">
                {client.description}
              </p>
              <CodeBlock
                code={client.install}
                language="shell"
                file={client.installLabel}
                onCopy={copyToClipboard}
              />
              <a
                href={client.repoUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="bg-secondary text-secondary-foreground hover:bg-secondary/80 inline-flex h-11 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors"
              >
                <ExternalLink className="size-4" />
                {client.repoName} on GitHub
              </a>
            </Card>
          </section>
        ))}
      </div>
    </div>
  )
}
