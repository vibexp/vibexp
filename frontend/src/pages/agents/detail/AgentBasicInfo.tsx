import { Bot, Pencil } from 'lucide-react'
import { useState } from 'react'

import { EmptyState } from '@/components/EmptyState'
import { Button } from '@/components/ui/button'
import type { Agent } from '@/services/agentService'

import { primaryInterface } from '../helpers'

interface AgentBasicInfoProps {
  agent: Agent
  /**
   * The page's own edit affordance, offered from the card-less empty state.
   * Omitted → no action is rendered; this component never re-derives whether
   * the viewer may edit.
   */
  onEdit?: () => void
}

/**
 * The article's identity strip: the agent's icon and the two facts that come
 * off its A2A card rather than off the resource row.
 *
 * The name, description, status badge and the Created / Updated timestamps this
 * block used to repeat are all rendered by the reading shell now (#918) — the
 * header carries the first three and the generated metadata section the rest —
 * so what is left is card-free, per the epic's article rule.
 *
 * With no card there is nothing card-derived to show, so the strip becomes an
 * empty state explaining why (#951) instead of a lone icon tile.
 */
export function AgentBasicInfo({
  agent,
  onEdit,
}: Readonly<AgentBasicInfoProps>) {
  const [iconLoadError, setIconLoadError] = useState(false)

  if (!agent.agent_card) {
    return (
      <div data-testid="agent-basic-info">
        <AgentCardEmptyState cardUrl={agent.card_url} onEdit={onEdit} />
      </div>
    )
  }

  return (
    <div className="flex items-center gap-4" data-testid="agent-basic-info">
      <div className="bg-muted flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-lg">
        {agent.agent_card.iconUrl && !iconLoadError ? (
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
      <div className="text-muted-foreground text-sm">
        {/* The card's own version is a generated "Card version" metadata row
            now, so repeating it here would put one value on screen under two
            labels — exactly the drift the descriptor removes. The protocol
            stays: it comes off `supportedInterfaces[0]`, an array index the
            descriptor's dotted-key reader cannot address. */}
        Protocol:{' '}
        {primaryInterface(agent.agent_card)?.protocolVersion ?? 'Not specified'}
      </div>
    </div>
  )
}

interface AgentCardEmptyStateProps {
  cardUrl?: string | null
  onEdit?: () => void
}

/**
 * Two different situations share a null `agent_card`: no card URL at all (a
 * name/description-only row, valid by the table's CHECK constraint), and a URL
 * whose card has never been fetched successfully. The copy deliberately does
 * not promise a background retry — that refresh is best-effort and silent on
 * failure — and points at Edit, whose save is what actually re-fetches.
 */
function AgentCardEmptyState({
  cardUrl,
  onEdit,
}: Readonly<AgentCardEmptyStateProps>) {
  const actions = onEdit ? (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={onEdit}
      data-testid="agent-card-empty-edit"
    >
      <Pencil className="size-4" />
      Edit agent
    </Button>
  ) : undefined

  if (cardUrl) {
    return (
      <EmptyState
        icon={Bot}
        title="Agent card not fetched yet"
        description={
          <>
            The A2A card at{' '}
            <code className="break-all" data-testid="agent-card-url">
              {cardUrl}
            </code>{' '}
            has not been fetched successfully, so there are no skills or
            capabilities to show. Saving the agent from Edit fetches it again
            and reports any error.
          </>
        }
        actions={actions}
        className="border-none p-8"
      />
    )
  }

  return (
    <EmptyState
      icon={Bot}
      title="No A2A card configured"
      description="This agent has no A2A agent card, so there are no skills or capabilities to show. Add a card URL from Edit to fetch one."
      actions={actions}
      className="border-none p-8"
    />
  )
}
