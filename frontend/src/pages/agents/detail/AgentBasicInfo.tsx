import { Bot, Calendar, Clock } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import type { Agent } from '@/types'

import { agentStatusLabel, agentStatusVariant, formatDate } from '../helpers'

interface AgentBasicInfoProps {
  agent: Agent
}

export function AgentBasicInfo({ agent }: AgentBasicInfoProps) {
  const [iconLoadError, setIconLoadError] = useState(false)

  return (
    <Card>
      <CardContent className="space-y-6 p-6">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-4">
            <div className="bg-muted flex size-16 items-center justify-center overflow-hidden rounded-lg">
              {agent.agent_card?.iconUrl && !iconLoadError ? (
                <img
                  src={agent.agent_card.iconUrl}
                  alt={agent.name}
                  className="size-full object-cover"
                  onError={() => {
                    setIconLoadError(true)
                  }}
                />
              ) : (
                <Bot className="text-muted-foreground size-8" />
              )}
            </div>
            <div>
              <h2 className="text-2xl font-semibold">{agent.name}</h2>
              {agent.description && (
                <p className="text-muted-foreground mt-1">
                  {agent.description}
                </p>
              )}
              {agent.agent_card && (
                <div className="text-muted-foreground mt-2 flex items-center gap-4 text-sm">
                  <span>Version: {agent.agent_card.version}</span>
                  <span>Protocol: {agent.agent_card.protocolVersion}</span>
                </div>
              )}
            </div>
          </div>
          <Badge variant={agentStatusVariant(agent.status)}>
            {agentStatusLabel(agent.status)}
          </Badge>
        </div>

        <Separator />

        <div className="text-muted-foreground flex flex-wrap items-center gap-6 text-sm">
          <div className="flex items-center gap-2">
            <Calendar className="size-4" />
            <span className="font-medium">Created:</span>
            <span>{formatDate(agent.created_at)}</span>
          </div>
          <div className="flex items-center gap-2">
            <Clock className="size-4" />
            <span className="font-medium">Updated:</span>
            <span>{formatDate(agent.updated_at)}</span>
          </div>
          {agent.last_synced_at && (
            <div className="flex items-center gap-2">
              <Calendar className="size-4" />
              <span className="font-medium">Synced:</span>
              <span>{formatDate(agent.last_synced_at)}</span>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
