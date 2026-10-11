import { toArtifactRequest } from '@/pages/artifacts/artifactRequest'
import { toBlueprintRequest } from '@/pages/blueprints/blueprintRequest'
import { toMemoryRequest } from '@/pages/memories/memoryRequest'
import { toPromptRequest } from '@/pages/prompts/promptRequest'

/*
 * A status is a system value or one of the team's custom statuses (#1263),
 * which only the server can check. The four request builders used to narrow it
 * against a hardcoded tuple and fall back to the first entry — so a custom
 * status picked in the form was silently saved as `active` (or `draft`).
 */
const BUILDERS: readonly (readonly [
  string,
  (status: string) => { status?: string },
])[] = [
  ['artifact', status => toArtifactRequest({ status })],
  ['blueprint', status => toBlueprintRequest({ status, type: 'general' })],
  ['memory', status => toMemoryRequest({ status }, [], 'omit')],
  ['prompt', status => toPromptRequest({ status }, false)],
]

describe.each(BUILDERS)('%s request status', (_kind, build) => {
  it.each(['active', 'draft', 'archived', 'published', 'expired'])(
    'sends the system status %s as the form holds it',
    status => {
      expect(build(status).status).toBe(status)
    }
  )

  it('sends a custom status slug unchanged', () => {
    expect(build('in-review').status).toBe('in-review')
  })

  it('sends a status the catalog no longer has unchanged, for the server to judge', () => {
    expect(build('deleted-elsewhere').status).toBe('deleted-elsewhere')
  })
})

describe('toPromptRequest — MCP exposure', () => {
  it('forces exposure off on a draft, whatever the toggle says', () => {
    expect(toPromptRequest({ status: 'draft' }, true).mcp_expose).toBe(false)
  })

  it.each(['published', 'in-review'])(
    'follows the toggle for the non-draft status %s',
    status => {
      expect(toPromptRequest({ status }, true).mcp_expose).toBe(true)
      expect(toPromptRequest({ status }, false).mcp_expose).toBe(false)
    }
  )
})
