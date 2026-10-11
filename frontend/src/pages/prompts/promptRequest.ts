import type { ResourceFormValues } from '@/components/patterns/resource'
import { stringListValue, stringValue } from '@/components/patterns/resource'
import type { CreatePromptRequest } from '@/services/promptService'

/**
 * The prompt request body, built from a generated form's parsed values plus the
 * page-owned MCP toggle.
 *
 * `status` is sent as the form holds it — see `toArtifactRequest`.
 *
 * `mcp_expose` is forced off on a draft rather than merely hidden: exposure is
 * only meaningful once a prompt has left draft — `published`, or any of the
 * team's custom statuses (#1263) — and the old settings pane cleared the flag
 * on every switch to draft. Doing it here instead makes the rule hold however
 * the status was reached, including a status the user never touched.
 */
export function toPromptRequest(
  values: ResourceFormValues,
  mcpExpose: boolean
): CreatePromptRequest {
  const status = stringValue(values, 'status')
  return {
    name: stringValue(values, 'name'),
    slug: stringValue(values, 'slug'),
    description: stringValue(values, 'description'),
    body: stringValue(values, 'body'),
    project_id: stringValue(values, 'project_id'),
    status,
    mcp_expose: status !== 'draft' && mcpExpose,
    labels: stringListValue(values, 'labels'),
  }
}
