import { Bot } from 'lucide-react'
import { useState } from 'react'

import type { Agent } from '@/services/agentService'

import { primaryInterface } from '../helpers'

interface AgentBasicInfoProps {
  agent: Agent
}

/**
 * The article's identity strip: the agent's icon and the two facts that come
 * off its A2A card rather than off the resource row.
 *
 * The name, description, status badge and the Created / Updated timestamps this
 * block used to repeat are all rendered by the reading shell now (#918) — the
 * header carries the first three and the generated metadata section the rest —
 * so what is left is card-free, per the epic's article rule.
 */
export function AgentBasicInfo({ agent }: Readonly<AgentBasicInfoProps>) {
  const [iconLoadError, setIconLoadError] = useState(false)

  return (
    <div className="flex items-center gap-4" data-testid="agent-basic-info">
      <div className="bg-muted flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-lg">
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
      {agent.agent_card && (
        <div className="text-muted-foreground text-sm">
          {/* The card's own version is a generated "Card version" metadata row
              now, so repeating it here would put one value on screen under two
              labels — exactly the drift the descriptor removes. The protocol
              stays: it comes off `supportedInterfaces[0]`, an array index the
              descriptor's dotted-key reader cannot address. */}
          Protocol:{' '}
          {primaryInterface(agent.agent_card)?.protocolVersion ??
            'Not specified'}
        </div>
      )}
    </div>
  )
}
