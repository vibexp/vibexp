import { mcpTools } from '../mcp-tools'
import { attachmentTools } from '../mcp-tools-attachment'
import { blueprintTools } from '../mcp-tools-blueprint'
import { memoryTools } from '../mcp-tools-memory'
import { metadataTools } from '../mcp-tools-metadata'
import { promptTools } from '../mcp-tools-prompt'
import { resourceTools } from '../mcp-tools-resource'

const EXPECTED_TOOL_NAMES = new Set([
  'vibexp_io_create_artifact',
  'vibexp_io_update_artifact',
  'vibexp_io_create_memory',
  'vibexp_io_update_memory',
  'vibexp_io_create_blueprint',
  'vibexp_io_update_blueprint',
  'vibexp_io_create_prompt',
  'vibexp_io_update_prompt',
  'vibexp_io_render_prompt',
  'vibexp_io_get_resource',
  'vibexp_io_list_resources',
  'vibexp_io_list_resource_metadata',
  'vibexp_io_delete_resource',
  'vibexp_io_upload_attachment',
  'vibexp_io_list_attachments',
  'vibexp_io_delete_attachment',
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
  it('contains exactly the 27 expected tool names', () => {
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

  // The curated catalog is hand-maintained and drifts silently from the backend
  // tool structs: it is what a user browsing /mcp actually reads. `labels`
  // (#910) and `title` (#911) were both added to the memory write tools, so
  // both are pinned here rather than trusted.
  //
  // Deliberately a SUBSET check, not an exhaustive one: `status` is a real
  // argument on both backend param structs and is missing from this catalog
  // already, so an exhaustive assertion would fail on a gap that predates the
  // two fields being pinned.
  it('documents title and labels on both memory write tools', () => {
    for (const name of ['vibexp_io_create_memory', 'vibexp_io_update_memory']) {
      const tool = mcpTools.find(t => t.name === name)
      expect(tool).toBeDefined()
      const properties = tool?.inputSchema.properties ?? {}
      expect(Object.keys(properties)).toEqual(
        expect.arrayContaining(['title', 'text', 'metadata', 'labels'])
      )
    }
  })

  // #937 added `labels` to the blueprint write tools, and #939 is what put
  // those tools in the catalog at all. `subtype` and `metadata` are the extras
  // most easily dropped when transcribing a Go param struct by hand, so they
  // are pinned rather than trusted.
  it('documents labels, subtype and metadata on both blueprint write tools', () => {
    for (const name of [
      'vibexp_io_create_blueprint',
      'vibexp_io_update_blueprint',
    ]) {
      const tool = mcpTools.find(t => t.name === name)
      expect(tool).toBeDefined()
      const properties = tool?.inputSchema.properties ?? {}
      expect(Object.keys(properties)).toEqual(
        expect.arrayContaining(['labels', 'subtype', 'metadata', 'content'])
      )
    }
  })

  it('documents relative_path on the attachment upload tool', () => {
    const tool = mcpTools.find(t => t.name === 'vibexp_io_upload_attachment')
    expect(tool).toBeDefined()
    expect(Object.keys(tool?.inputSchema.properties ?? {})).toEqual(
      expect.arrayContaining([
        'owner_type',
        'owner_id',
        'file_name',
        'file_content_base64',
        'relative_path',
      ])
    )
    expect(tool?.inputSchema.required).not.toContain('relative_path')
  })

  // Belt and braces alongside `TestMCPCatalogModulesAreAllSpread`, which makes
  // the same assertion on the Go side. That one reads the modules as TEXT and
  // so covers every module by construction; this one reads the RUNTIME arrays,
  // so it is the half that would notice a spread that parses but resolves to
  // something else.
  it('spreads every per-domain module into the rendered catalog', () => {
    const names = new Set(mcpTools.map(t => t.name))
    const modules = {
      memoryTools,
      metadataTools,
      blueprintTools,
      promptTools,
      attachmentTools,
      resourceTools,
    }
    for (const [moduleName, tools] of Object.entries(modules)) {
      expect(tools.length).toBeGreaterThan(0)
      for (const tool of tools) {
        expect(
          names.has(tool.name),
          `${tool.name} is exported by ${moduleName} but is not spread into mcpTools`
        ).toBe(true)
      }
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
