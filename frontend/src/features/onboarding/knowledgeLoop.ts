/**
 * When the signed-in user's AI tools first read from and wrote back to the
 * shared brain (`CurrentUser.knowledge_loop`, #1509). Only reads and writes
 * from an AI tool count (cli, mcp, api). Null until it happens.
 */
export interface KnowledgeLoop {
  first_read_at: string | null
  first_write_at: string | null
}

const NOT_YET: KnowledgeLoop = { first_read_at: null, first_write_at: null }

/**
 * The user's knowledge loop; a backend that does not send it (older than
 * #1509) reads as not yet.
 */
export function knowledgeLoopOf(
  user: object | null | undefined
): KnowledgeLoop {
  if (!user || !('knowledge_loop' in user)) return NOT_YET
  const loop = user.knowledge_loop as Partial<KnowledgeLoop> | null | undefined
  return {
    first_read_at: loop?.first_read_at ?? null,
    first_write_at: loop?.first_write_at ?? null,
  }
}

/** The loop is running once an agent has both read and written back. */
export function isLoopRunning(loop: KnowledgeLoop): boolean {
  return loop.first_read_at !== null && loop.first_write_at !== null
}
