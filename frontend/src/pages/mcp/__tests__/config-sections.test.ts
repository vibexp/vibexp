import {
  buildMcpServerName,
  getConfigSections,
  MCP_ENDPOINT,
} from '../config-sections'

describe('buildMcpServerName', () => {
  it('slugifies a multi-word personal workspace name', () => {
    expect(buildMcpServerName('Private Workspace')).toBe(
      'vibexp_io_private_workspace'
    )
  })

  it('lowercases and joins words with a single underscore', () => {
    expect(buildMcpServerName('vibexp team')).toBe('vibexp_io_vibexp_team')
  })

  it('collapses runs of special characters into a single underscore', () => {
    expect(buildMcpServerName('Acme & Co. 2!')).toBe('vibexp_io_acme_co_2')
  })

  it('trims leading and trailing separators', () => {
    expect(buildMcpServerName('  --Hello--  ')).toBe('vibexp_io_hello')
  })

  it('falls back to workspace when the name yields an empty slug', () => {
    expect(buildMcpServerName('!@# $%^')).toBe('vibexp_io_workspace')
  })

  it('falls back to workspace for an empty string', () => {
    expect(buildMcpServerName('')).toBe('vibexp_io_workspace')
  })

  it('falls back to workspace for an emoji-only name', () => {
    expect(buildMcpServerName('🚀✨')).toBe('vibexp_io_workspace')
  })
})

describe('getConfigSections', () => {
  const serverName = 'vibexp_io_vibexp_team'
  const sections = getConfigSections(serverName)

  it('exposes the single team-agnostic MCP endpoint', () => {
    expect(MCP_ENDPOINT).toBe('https://connect.vibexp.io/mcp/v1/common')
  })

  it('returns four configuration sections', () => {
    expect(sections).toHaveLength(4)
    expect(sections.map(s => s.id)).toEqual([
      'claude-code',
      'cursor',
      'vscode',
      'gemini',
    ])
  })

  it('interpolates the dynamic server name into every snippet', () => {
    for (const section of sections) {
      expect(section.code).toContain(serverName)
    }
  })

  it('never references the hardcoded vibexp_io_common name', () => {
    for (const section of sections) {
      expect(section.code).not.toContain('vibexp_io_common')
    }
  })

  it('removes the stray "github" server key from the VSCode snippet', () => {
    const vscode = sections.find(s => s.id === 'vscode')
    expect(vscode?.code).not.toContain('"github"')
  })

  it('uses the dynamic server key in the VSCode snippet', () => {
    const vscode = sections.find(s => s.id === 'vscode')
    expect(vscode?.code).toContain(`"${serverName}": {`)
  })

  it('uses the single team-agnostic endpoint in every snippet', () => {
    for (const section of sections) {
      expect(section.code).toContain(MCP_ENDPOINT)
    }
  })

  it('no longer encodes a team id in the endpoint path', () => {
    for (const section of sections) {
      expect(section.code).not.toContain('/teams/')
    }
  })

  it('never references API-key authentication in any snippet', () => {
    const forbidden = [
      'Authorization',
      'Bearer',
      'API_KEY',
      'api_key',
      'headers',
      'inputs',
    ]
    for (const section of sections) {
      for (const token of forbidden) {
        expect(section.code).not.toContain(token)
        expect(section.description).not.toContain(token)
      }
    }
  })

  it('frames every description around browser sign-in', () => {
    for (const section of sections) {
      expect(section.description.toLowerCase()).toContain('sign in to vibexp')
    }
  })

  it('uses the httpUrl key for the Gemini snippet', () => {
    const gemini = sections.find(s => s.id === 'gemini')
    expect(gemini?.code).toContain(`"httpUrl": "${MCP_ENDPOINT}"`)
    expect(gemini?.code).not.toContain('"url"')
  })

  it('uses a plain url for the cursor and vscode snippets', () => {
    const cursor = sections.find(s => s.id === 'cursor')
    const vscode = sections.find(s => s.id === 'vscode')
    expect(cursor?.code).toContain(`"url": "${MCP_ENDPOINT}"`)
    expect(vscode?.code).toContain(`"url": "${MCP_ENDPOINT}"`)
  })

  it('adds the server with no --header flag for Claude Code', () => {
    const claudeCode = sections.find(s => s.id === 'claude-code')
    expect(claudeCode?.code).toBe(
      `claude mcp add --transport http ${serverName} ${MCP_ENDPOINT}`
    )
  })
})
