import type { components } from '@vibexp/api-client'

import type { CurrentUser } from '@/services/authService'

/**
 * When the signed-in user's AI tools first read from and wrote back to the
 * shared brain (`CurrentUser.knowledge_loop`). Only reads and writes from an
 * AI tool count (cli, mcp, api). Null until it happens.
 */
export type KnowledgeLoop = components['schemas']['KnowledgeLoop']

const NOT_YET: KnowledgeLoop = { first_read_at: null, first_write_at: null }

/** The signed-in user's knowledge loop; nothing has happened without a user. */
export function knowledgeLoopOf(user: CurrentUser | null): KnowledgeLoop {
  return user?.knowledge_loop ?? NOT_YET
}

/** The loop is running once an agent has both read and written back. */
export function isLoopRunning(loop: KnowledgeLoop): boolean {
  return loop.first_read_at !== null && loop.first_write_at !== null
}
