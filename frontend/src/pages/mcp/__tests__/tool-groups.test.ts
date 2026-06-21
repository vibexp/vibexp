import type { MCPTool } from '../mcp-tools'
import { mcpTools } from '../mcp-tools'
import { getToolKind, getToolSummary, groupTools } from '../tool-groups'

function makeTool(name: string, description = 'Does a thing.'): MCPTool {
  return {
    name,
    description,
    inputSchema: {
      type: 'object',
      properties: {},
      required: [],
      additionalProperties: false,
    },
  }
}

describe('getToolKind', () => {
  it('classifies create/update/post/reply tools as writes', () => {
    expect(getToolKind(makeTool('vibexp_io_create_artifact'))).toBe('write')
    expect(getToolKind(makeTool('vibexp_io_update_memory'))).toBe('write')
    expect(getToolKind(makeTool('vibexp_io_post_to_feed'))).toBe('write')
    expect(getToolKind(makeTool('vibexp_io_reply_to_feed_item'))).toBe('write')
  })

  it('classifies search/get/list tools as reads', () => {
    expect(getToolKind(makeTool('vibexp_io_search_artifacts'))).toBe('read')
    expect(getToolKind(makeTool('vibexp_io_get_memory'))).toBe('read')
    expect(getToolKind(makeTool('vibexp_io_list_teams'))).toBe('read')
  })
})

describe('getToolSummary', () => {
  it('returns the first sentence of the description', () => {
    expect(
      getToolSummary(makeTool('x', 'Create a new artifact. More detail here.'))
    ).toBe('Create a new artifact.')
  })

  it('truncates a long description with no early sentence break', () => {
    const long = 'a'.repeat(200)
    const summary = getToolSummary(makeTool('x', long))
    expect(summary.endsWith('…')).toBe(true)
    expect(summary.length).toBeLessThan(long.length)
  })
})

describe('groupTools', () => {
  it('buckets tools into ordered, labelled groups', () => {
    const groups = groupTools(mcpTools)
    expect(groups.map(g => g.id)).toEqual([
      'artifacts',
      'memories',
      'projects-feeds',
      'teams',
      'account',
    ])
  })

  it('places every tool in exactly one group', () => {
    const groups = groupTools(mcpTools)
    const grouped = groups.flatMap(g => g.tools)
    expect(grouped).toHaveLength(mcpTools.length)
    expect(new Set(grouped.map(t => t.name)).size).toBe(mcpTools.length)
  })

  it('routes artifact and memory tools to their dedicated groups', () => {
    const groups = groupTools(mcpTools)
    const artifacts = groups.find(g => g.id === 'artifacts')
    const memories = groups.find(g => g.id === 'memories')
    expect(artifacts?.tools.every(t => t.name.includes('_artifact'))).toBe(true)
    expect(memories?.tools.every(t => t.name.includes('_memor'))).toBe(true)
  })

  it('omits groups with no matching tools', () => {
    const groups = groupTools([makeTool('vibexp_io_list_teams')])
    expect(groups.map(g => g.id)).toEqual(['teams'])
  })

  it('drops tools filtered out before grouping', () => {
    const groups = groupTools([])
    expect(groups).toHaveLength(0)
  })
})
