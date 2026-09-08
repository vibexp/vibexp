import type { ResourceFormValues } from '@/components/patterns/resource'
import {
  enumValue,
  stringListValue,
  stringValue,
} from '@/components/patterns/resource'
import type { CreatePromptRequest, Prompt } from '@/services/promptService'

type PromptStatus = Prompt['status']

const PROMPT_STATUSES: readonly [PromptStatus, ...PromptStatus[]] = [
  'draft',
  'published',
]

/**
 * The prompt request body, built from a generated form's parsed values plus the
 * page-owned MCP toggle.
 *
 * `mcp_expose` is forced off on a draft rather than merely hidden: exposure is
 * only meaningful once a prompt is published, and the old settings pane cleared
 * the flag on every switch to draft. Doing it here instead makes the rule hold
 * however the status was reached, including a status the user never touched.
 */
export function toPromptRequest(
  values: ResourceFormValues,
  mcpExpose: boolean
): CreatePromptRequest {
  const status = enumValue(values, 'status', PROMPT_STATUSES)
  return {
    name: stringValue(values, 'name'),
    slug: stringValue(values, 'slug'),
    description: stringValue(values, 'description'),
    body: stringValue(values, 'body'),
    project_id: stringValue(values, 'project_id'),
    status,
    mcp_expose: status === 'published' && mcpExpose,
    labels: stringListValue(values, 'labels'),
  }
}
