import { MCP_ENDPOINT } from '../../config/siteConfig'

export interface ConfigSection {
  id: string
  title: string
  description: string
  code: string
  language: string
  /** Where the snippet goes — a terminal, or the client's MCP config file. */
  file: string
}

/**
 * Builds an MCP-safe server name from a workspace/team display name. The result
 * is a local label in the user's MCP client config and never affects routing —
 * a distinct name per workspace lets users register multiple VibeXP servers in
 * the same client without colliding keys.
 */
export function buildMcpServerName(teamName: string): string {
  const slug = teamName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
  return `vibexp_io_${slug || 'workspace'}`
}

/**
 * Re-exported for backwards compatibility. The single, team-agnostic MCP
 * endpoint is configured via `VITE_MCP_ENDPOINT` (see config/siteConfig.ts).
 * Team is no longer encoded in the URL; the AI client passes a team_id (team
 * UUID or slug) per tool call and can discover the user's teams via the
 * list-teams tool.
 */
export { MCP_ENDPOINT }

export function getConfigSections(serverName: string): ConfigSection[] {
  return [
    {
      id: 'claude-code',
      title: 'Claude Code CLI',
      description:
        'Add the VibeXP MCP server to Claude Code. The CLI opens your browser to sign in to VibeXP — no API key needed.',
      language: 'bash',
      file: 'Terminal',
      code: `claude mcp add --transport http ${serverName} ${MCP_ENDPOINT}`,
    },
    {
      id: 'cursor',
      title: 'Cursor IDE',
      description:
        'Add this to your Cursor MCP settings. Cursor opens your browser to sign in to VibeXP on first use — no API key needed.',
      language: 'json',
      file: '~/.cursor/mcp.json',
      code: `{
  "mcpServers": {
    "${serverName}": {
      "url": "${MCP_ENDPOINT}"
    }
  }
}`,
    },
    {
      id: 'vscode',
      title: 'VSCode',
      description:
        'Add this to your VSCode MCP settings. VSCode opens your browser to sign in to VibeXP on first use — no API key needed.',
      language: 'json',
      file: '.vscode/mcp.json',
      code: `{
  "servers": {
    "${serverName}": {
      "type": "http",
      "url": "${MCP_ENDPOINT}"
    }
  }
}`,
    },
    {
      id: 'gemini',
      title: 'Gemini CLI',
      description:
        'Add this to your Gemini CLI MCP settings. Gemini opens your browser to sign in to VibeXP on first use — no API key needed.',
      language: 'json',
      file: '~/.gemini/settings.json',
      code: `{
  "mcpServers": {
    "${serverName}": {
      "httpUrl": "${MCP_ENDPOINT}"
    }
  }
}`,
    },
  ]
}
