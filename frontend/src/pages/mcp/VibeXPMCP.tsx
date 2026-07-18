import { AlertTriangle, Check, Copy, Info, Plug } from 'lucide-react'
import { useEffect, useState } from 'react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTeam } from '@/contexts/TeamContext'
import { useAlerts, useAnalytics } from '@/hooks'
import { cn } from '@/lib/utils'
import { CodeBlock } from '@/pages/mcp/CodeBlock'
import type { ConfigSection } from '@/pages/mcp/config-sections'
import {
  buildMcpServerName,
  getConfigSections,
  MCP_ENDPOINT,
} from '@/pages/mcp/config-sections'
import { mcpTools } from '@/pages/mcp/mcp-tools'
import { MCPToolsList } from '@/pages/mcp/MCPToolsList'
import { TeamIdentifiers } from '@/pages/mcp/TeamIdentifiers'
import { ANALYTICS_EVENTS } from '@/types/analytics'

const STEPS = [
  {
    title: 'Copy the endpoint',
    desc: 'One HTTP endpoint serves every team — paste it into your AI client.',
  },
  {
    title: 'Add it to your client',
    desc: 'Pick your tool below and drop the config into its MCP settings.',
  },
  {
    title: 'Sign in & pick a team',
    desc: 'Your client opens the browser to authorize. Pass a team_id per call.',
  },
]

function SectionHeading({
  title,
  description,
}: Readonly<{
  title: string
  description?: React.ReactNode
}>) {
  return (
    <div className="mb-4">
      <div className="text-lg font-semibold tracking-tight">{title}</div>
      {description && (
        <p className="text-muted-foreground mt-1 max-w-[70ch] text-sm">
          {description}
        </p>
      )}
    </div>
  )
}

function Mono({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <code className="bg-secondary rounded-sm px-1.5 py-0.5 font-mono text-sm">
      {children}
    </code>
  )
}

export function VibeXPMCP() {
  const { showSuccess } = useAlerts()
  const { trackEvent } = useAnalytics()
  const { currentTeam, teams, isLoading: isLoadingTeam } = useTeam()

  const [activeTab, setActiveTab] = useState<string>('claude-code')
  const [expandedTools, setExpandedTools] = useState<Set<string>>(new Set())
  const [endpointCopied, setEndpointCopied] = useState(false)

  useEffect(() => {
    trackEvent({
      event: ANALYTICS_EVENTS.MCP_INTEGRATION_PAGE_VIEW,
      properties: { action_context: 'view' },
    })
  }, [trackEvent])

  const handleTabChange = (tabId: string) => {
    setActiveTab(tabId)
    trackEvent({
      event: ANALYTICS_EVENTS.MCP_CONFIG_SECTION_EXPANDED,
      properties: { section_id: tabId, action_context: 'expand' },
    })
  }

  const toggleTool = (toolName: string) => {
    const isExpanded = expandedTools.has(toolName)
    setExpandedTools(prev => {
      const next = new Set(prev)
      if (next.has(toolName)) {
        next.delete(toolName)
      } else {
        next.add(toolName)
      }
      return next
    })
    if (!isExpanded) {
      trackEvent({
        event: ANALYTICS_EVENTS.MCP_TOOL_EXPANDED,
        properties: { tool_name: toolName, action_context: 'expand' },
      })
    }
  }

  const copyToClipboard = (text: string, label: string) => {
    void navigator.clipboard.writeText(text).then(() => {
      trackEvent({
        event: ANALYTICS_EVENTS.MCP_CONFIG_COPIED,
        properties: { config_type: label, action_context: 'copy' },
      })
      showSuccess(`${label} configuration copied`, 'Copied')
    })
  }

  const copyEndpoint = () => {
    copyToClipboard(MCP_ENDPOINT, 'Endpoint')
    setEndpointCopied(true)
    setTimeout(() => {
      setEndpointCopied(false)
    }, 1500)
  }

  if (isLoadingTeam) {
    return (
      <div className="space-y-6">
        <PageHeader title="Loading…" />
        <div className="flex justify-center py-12">
          <LoadingSpinner size="lg" label="Loading team information…" />
        </div>
      </div>
    )
  }

  if (!currentTeam) {
    return (
      <div className="space-y-6">
        <PageHeader title="VibeXP MCP Integration" />
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>No team available</AlertTitle>
          <AlertDescription>
            You need to be part of a team to configure MCP integration. Please
            create or join a team first.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const serverName = buildMcpServerName(currentTeam.name)
  const configSections: ConfigSection[] = getConfigSections(serverName)

  return (
    <div className="mx-auto max-w-[1080px]">
      {/* header */}
      <header className="mb-8">
        <h1 className="flex items-center gap-3 text-3xl font-bold tracking-tight">
          <span className="bg-primary text-primary-foreground grid size-[38px] place-items-center rounded-md">
            <Plug className="size-[21px]" />
          </span>
          VibeXP MCP Integration
        </h1>
        <p className="text-muted-foreground mt-2.5 text-base">
          Connect VibeXP to your favorite AI tools over the Model Context
          Protocol — three steps, no API key.
        </p>
      </header>

      {/* steps */}
      <Card className="grid grid-cols-1 gap-0 p-0 sm:grid-cols-3">
        {STEPS.map((step, i) => (
          <div
            key={step.title}
            className={cn(
              'p-[22px]',
              i > 0 && 'sm:border-l',
              i > 0 && 'border-t sm:border-t-0'
            )}
          >
            <div className="bg-primary text-primary-foreground mb-[13px] grid size-[30px] place-items-center rounded-full text-sm font-bold tabular-nums">
              {i + 1}
            </div>
            <div className="text-base font-semibold tracking-tight">
              {step.title}
            </div>
            <p className="text-muted-foreground mt-[5px] text-sm">
              {step.desc}
            </p>
          </div>
        ))}
      </Card>

      {/* endpoint */}
      <section className="mt-9">
        <SectionHeading title="Endpoint" />
        <div className="flex flex-wrap items-center gap-3.5">
          <div className="border-input bg-muted flex min-w-[280px] flex-1 items-center gap-3 rounded-md border py-3 pl-4 pr-2">
            <span className="text-muted-foreground bg-background rounded-full border px-[7px] py-[3px] text-xs font-bold uppercase tracking-wider">
              HTTP
            </span>
            <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-base font-medium">
              {MCP_ENDPOINT}
            </span>
          </div>
          <button
            type="button"
            onClick={copyEndpoint}
            className="bg-secondary text-secondary-foreground hover:bg-secondary/80 inline-flex h-11 items-center gap-2 rounded-md px-4 text-sm font-medium transition-colors"
          >
            {endpointCopied ? (
              <Check className="size-4" />
            ) : (
              <Copy className="size-4" />
            )}
            {endpointCopied ? 'Copied' : 'Copy endpoint'}
          </button>
        </div>
        <div className="border-info/20 bg-info-subtle mt-3.5 grid grid-cols-[18px_1fr] items-start gap-x-3 rounded-lg border p-4 text-sm">
          <Info className="text-info mt-0.5 size-[17px]" />
          <div>
            A single endpoint now serves <b>every</b> team. If you previously
            used a team-specific URL like{' '}
            <Mono>…/teams/&lt;uuid&gt;/common</Mono>, switch to the common
            endpoint above — your agent passes a <Mono>team_id</Mono> (UUID or
            slug) on each tool call instead. Grab yours below.
          </div>
        </div>
      </section>

      {/* connect client */}
      <section className="mt-9">
        <SectionHeading
          title="Connect your client"
          description="Choose your AI tool and add the configuration to its MCP settings. Each client handles the OAuth sign-in for you on first connect."
        />
        {/* The shadcn/Radix Tabs primitive gives us the segmented look plus
            correct tab/tabpanel ARIA wiring and arrow-key navigation. */}
        <Tabs value={activeTab} onValueChange={handleTabChange}>
          <TabsList className="h-auto flex-wrap justify-start gap-0.5">
            {configSections.map(section => (
              <TabsTrigger
                key={section.id}
                value={section.id}
                className="px-3.5 py-[7px] text-sm data-[state=active]:font-semibold"
              >
                {section.title}
              </TabsTrigger>
            ))}
          </TabsList>
          {configSections.map(section => (
            <TabsContent key={section.id} value={section.id} className="mt-0">
              <p className="text-muted-foreground my-3 max-w-[70ch] text-sm">
                {section.description}
              </p>
              <CodeBlock
                code={section.code}
                language={section.language}
                file={section.file}
                onCopy={code => {
                  copyToClipboard(code, section.title)
                }}
              />
            </TabsContent>
          ))}
        </Tabs>
      </section>

      {/* teams */}
      <section className="mt-9">
        <SectionHeading
          title="Your team identifiers"
          description={
            <>
              Hand one of these to your agent as the <Mono>team_id</Mono> on
              each tool call — or let it discover teams with the{' '}
              <Mono>vibexp_io_list_teams</Mono> tool.
            </>
          }
        />
        <TeamIdentifiers teams={teams} />
      </section>

      {/* tools */}
      <section className="mb-20 mt-9">
        <SectionHeading
          title="Tools reference"
          description={`The ${String(mcpTools.length)} tools your agent can call once connected. Expand any tool for its parameters.`}
        />
        <MCPToolsList
          tools={mcpTools}
          expandedTools={expandedTools}
          onToggleTool={toggleTool}
        />
      </section>
    </div>
  )
}
