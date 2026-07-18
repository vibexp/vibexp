import { ExternalLink, Globe, Shield, Tag, Zap } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import type { Agent } from '@/services/agentService'

import { primaryInterface } from '../helpers'

interface AgentCardDetailsProps {
  agent: Agent
}

function MetaTile({
  icon: Icon,
  label,
  value,
}: Readonly<{
  icon: typeof Globe
  label: string
  value: string
}>) {
  return (
    <div className="bg-muted rounded-md p-3">
      <div className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
        <Icon className="size-4" />
        {label}
      </div>
      <div className="mt-1 text-sm font-medium">{value}</div>
    </div>
  )
}

export function AgentCardDetails({ agent }: Readonly<AgentCardDetailsProps>) {
  if (!agent.agent_card) return null

  const iface = primaryInterface(agent.agent_card)

  return (
    <Card>
      <CardContent className="space-y-6 p-6">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Agent card details</h3>
          {agent.card_url && (
            <a
              href={agent.card_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary inline-flex items-center gap-1 text-sm hover:underline"
            >
              <ExternalLink className="size-4" />
              View card
            </a>
          )}
        </div>

        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          <MetaTile
            icon={Globe}
            label="Transport"
            value={iface?.protocolBinding ?? 'Not specified'}
          />
          <MetaTile
            icon={Zap}
            label="Streaming"
            value={
              agent.agent_card.capabilities?.streaming ? 'Enabled' : 'Disabled'
            }
          />
          <MetaTile
            icon={Shield}
            label="Skills"
            value={`${String(agent.agent_card.skills?.length ?? 0)} available`}
          />
        </div>

        <Separator />

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <h4 className="mb-2 text-sm font-medium">Input modes</h4>
            <div className="flex flex-wrap gap-1.5">
              {(agent.agent_card.defaultInputModes ?? []).map(mode => (
                <Badge key={mode} variant="secondary">
                  {mode}
                </Badge>
              ))}
            </div>
          </div>
          <div>
            <h4 className="mb-2 text-sm font-medium">Output modes</h4>
            <div className="flex flex-wrap gap-1.5">
              {(agent.agent_card.defaultOutputModes ?? []).map(mode => (
                <Badge key={mode} variant="secondary">
                  {mode}
                </Badge>
              ))}
            </div>
          </div>
        </div>

        {(agent.agent_card.skills?.length ?? 0) > 0 && (
          <>
            <Separator />
            <div>
              <h4 className="mb-3 text-sm font-medium">
                Skills &amp; capabilities
              </h4>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                {(agent.agent_card.skills ?? []).slice(0, 6).map(skill => (
                  <div
                    key={skill.id ?? skill.name}
                    className="bg-muted/50 rounded-md border p-3"
                  >
                    <div className="font-medium">{skill.name}</div>
                    {skill.description && (
                      <p className="text-muted-foreground mt-1 text-sm">
                        {skill.description}
                      </p>
                    )}
                    {(skill.tags?.length ?? 0) > 0 && (
                      <div className="mt-2 flex flex-wrap gap-1">
                        {(skill.tags ?? []).slice(0, 3).map(tag => (
                          <Badge key={tag} variant="outline" className="gap-1">
                            <Tag className="size-3" />
                            {tag}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
                {(agent.agent_card.skills?.length ?? 0) > 6 && (
                  <p className="text-muted-foreground py-3 text-center text-sm md:col-span-2">
                    +{(agent.agent_card.skills?.length ?? 0) - 6} more skills
                  </p>
                )}
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
