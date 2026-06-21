import { mcpTools } from '../mcp-tools'

const EXPECTED_TOOL_NAMES = new Set([
  'vibexp_io_create_artifact',
  'vibexp_io_search_artifacts',
  'vibexp_io_get_artifact',
  'vibexp_io_update_artifact',
  'vibexp_io_create_memory',
  'vibexp_io_search_memories',
  'vibexp_io_get_memory',
  'vibexp_io_update_memory',
  'vibexp_io_list_projects',
  'vibexp_io_list_feeds',
  'vibexp_io_list_feed_items',
  'vibexp_io_get_feed_item',
  'vibexp_io_list_feed_item_replies',
  'vibexp_io_get_feed_item_reply',
  'vibexp_io_post_to_feed',
  'vibexp_io_reply_to_feed_item',
  'vibexp_io_get_user',
  'vibexp_io_search',
  'vibexp_io_list_teams',
])

describe('mcpTools catalog', () => {
  it('contains exactly the 19 expected tool names', () => {
    const actualNames = new Set(mcpTools.map(t => t.name))
    expect(actualNames).toEqual(EXPECTED_TOOL_NAMES)
  })

  it('includes the team discovery tool vibexp_io_list_teams', () => {
    const names = new Set(mcpTools.map(t => t.name))
    expect(names).toContain('vibexp_io_list_teams')
  })

  it('every entry has non-empty name and description', () => {
    for (const tool of mcpTools) {
      expect(tool.name.trim()).not.toBe('')
      expect(tool.description.trim()).not.toBe('')
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

  it('get_feed_item and get_feed_item_reply are present', () => {
    const names = new Set(mcpTools.map(t => t.name))
    expect(names).toContain('vibexp_io_get_feed_item')
    expect(names).toContain('vibexp_io_get_feed_item_reply')
  })
})
