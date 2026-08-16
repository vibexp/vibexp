import { mcpTools } from '../mcp-tools'

const EXPECTED_TOOL_NAMES = new Set([
  'vibexp_io_create_artifact',
  'vibexp_io_update_artifact',
  'vibexp_io_create_memory',
  'vibexp_io_update_memory',
  'vibexp_io_get_resource',
  'vibexp_io_list_resources',
  'vibexp_io_list_resource_metadata',
  'vibexp_io_link_resources',
  'vibexp_io_list_projects',
  'vibexp_io_list_feeds',
  'vibexp_io_list_feed_items',
  'vibexp_io_get_feed_item',
  'vibexp_io_post_to_feed',
  'vibexp_io_reply_to_feed_item',
  'vibexp_io_get_user',
  'vibexp_io_search',
  'vibexp_io_list_teams',
  'vibexp_io_list_teams_and_projects',
])

describe('mcpTools catalog', () => {
  it('contains exactly the 18 expected tool names', () => {
    const actualNames = new Set(mcpTools.map(t => t.name))
    expect(actualNames).toEqual(EXPECTED_TOOL_NAMES)
  })

  it('includes the workspace discovery tool vibexp_io_list_teams_and_projects', () => {
    const names = new Set(mcpTools.map(t => t.name))
    expect(names).toContain('vibexp_io_list_teams_and_projects')
  })

  it('still lists the deprecated aliases, with wording naming their replacement', () => {
    // They survive one release (#814): removing them outright breaks agents
    // mid-session. The deprecation wording is the only signal an already-running
    // agent gets, so it is asserted rather than assumed.
    for (const name of ['vibexp_io_list_teams', 'vibexp_io_list_projects']) {
      const tool = mcpTools.find(t => t.name === name)
      expect(tool).toBeDefined()
      expect(tool?.description).toContain('DEPRECATED')
      expect(tool?.description).toContain('vibexp_io_list_teams_and_projects')
    }
  })

  it('every entry has non-empty name and description', () => {
    for (const tool of mcpTools) {
      expect(tool.name.trim()).not.toBe('')
      expect(tool.description.trim()).not.toBe('')
    }
  })

  it('classifies every tool by how it takes team_id', () => {
    // Three buckets, not two. The split used to be binary — a tool either
    // required team_id or had none — but vibexp_io_list_teams_and_projects is
    // deliberately both: it is the discovery entry point, so it MUST be callable
    // before any team is known, and it accepts a team_id to narrow the search
    // once one is. Classifying it as plain "user-scoped" would wrongly assert it
    // has no team_id property at all; classifying it as team-scoped would
    // wrongly require one. Hence a bucket of its own (#815).
    const userScoped = new Set(['vibexp_io_get_user', 'vibexp_io_list_teams'])
    const optionalTeam = new Set(['vibexp_io_list_teams_and_projects'])

    for (const tool of mcpTools) {
      if (userScoped.has(tool.name)) {
        expect(tool.inputSchema.properties).not.toHaveProperty('team_id')
        expect(tool.inputSchema.required).not.toContain('team_id')
      } else if (optionalTeam.has(tool.name)) {
        expect(tool.inputSchema.properties).toHaveProperty('team_id')
        expect(tool.inputSchema.required).not.toContain('team_id')
      } else {
        expect(tool.inputSchema.properties).toHaveProperty('team_id')
        expect(tool.inputSchema.required).toContain('team_id')
      }
    }
  })

  it('every entry has additionalProperties === false', () => {
    for (const tool of mcpTools) {
      expect(tool.inputSchema.additionalProperties).toBe(false)
    }
  })

  it('every entry has inputSchema.type === "object"', () => {
    for (const tool of mcpTools) {
      expect(tool.inputSchema.type).toBe('object')
    }
  })

  it('every required key exists in properties', () => {
    for (const tool of mcpTools) {
      for (const key of tool.inputSchema.required) {
        expect(tool.inputSchema.properties).toHaveProperty(key)
      }
    }
  })

  it('vibexp_io_current_date_time is not in the catalog (removed tool)', () => {
    const names = mcpTools.map(t => t.name)
    expect(names).not.toContain('vibexp_io_current_date_time')
  })

  it('exposes the unified resource read tools and drops the per-type read tools', () => {
    const names = new Set(mcpTools.map(t => t.name))
    expect(names).toContain('vibexp_io_get_resource')
    expect(names).toContain('vibexp_io_list_resources')
    // Consolidated away in epic #259.
    expect(names).not.toContain('vibexp_io_get_artifact')
    expect(names).not.toContain('vibexp_io_search_artifacts')
    expect(names).not.toContain('vibexp_io_get_memory')
    expect(names).not.toContain('vibexp_io_search_memories')
  })

  it('get_feed_item is present and the split reply-read tools are gone', () => {
    const names = new Set(mcpTools.map(t => t.name))
    expect(names).toContain('vibexp_io_get_feed_item')
    expect(names).not.toContain('vibexp_io_get_feed_item_reply')
    expect(names).not.toContain('vibexp_io_list_feed_item_replies')
  })
})
