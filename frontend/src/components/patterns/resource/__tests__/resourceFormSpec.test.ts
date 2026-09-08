import type { ResourceKindKey } from '../registry'
import { getResourceDescriptor } from '../registry'
import type { FormFieldSpec } from '../types'
import { requestLimits } from './specYaml'

/**
 * A generated form's field bounds must not be laxer than the API's.
 *
 * `descriptor.form` moved the max lengths and list caps out of four
 * hand-written zod schemas and into data (#913), which fixes them drifting
 * from *each other* — this fixes them drifting from the **spec**, which is the
 * failure that actually reaches a user: a value the form accepts and the API
 * answers with a 400. It caught one on the first run — the prompt `name` is
 * capped at 50, not the 255 every other kind's identifier gets.
 *
 * Read out of `backend/schemas/*.yaml` rather than restated, per CLAUDE.md's
 * spec-first rule and the precedent in `resourceListSpec.test.ts`.
 */
const SCHEMA_FILE: Partial<Record<ResourceKindKey, [string, string]>> = {
  artifact: ['artifacts.yaml', 'CreateArtifactRequest'],
  blueprint: ['blueprints.yaml', 'CreateBlueprintRequest'],
  memory: ['memories.yaml', 'CreateMemoryRequest'],
  prompt: ['prompts.yaml', 'CreatePromptRequest'],
}

const KINDS = Object.entries(SCHEMA_FILE) as [
  ResourceKindKey,
  [string, string],
][]

function formFields(kind: ResourceKindKey): readonly FormFieldSpec[] {
  return getResourceDescriptor(kind).form?.fields ?? []
}

describe('resource form specs', () => {
  it.each(KINDS)('%s declares a form section', kind => {
    expect(formFields(kind).length).toBeGreaterThan(0)
  })

  it('the read-only gallery kind declares none', () => {
    expect(getResourceDescriptor('gallery-prompt').form).toBeUndefined()
  })

  it('caps every text field no laxer than its create request does', () => {
    const checked: string[] = []
    for (const [kind, [file, request]] of KINDS) {
      for (const spec of formFields(kind)) {
        if (spec.control !== 'text' && spec.control !== 'textarea') continue
        const limits = requestLimits(file, request, spec.key)
        if (limits.maxLength === undefined) continue
        checked.push(`${kind}.${spec.key}`)
        expect(spec.maxLength).toBeDefined()
        expect(spec.maxLength).toBeLessThanOrEqual(limits.maxLength)
      }
    }
    // Guards the guard: a lookup that silently found nothing would make every
    // assertion above vacuous. Memory contributes none — its only long field
    // is a `body` control, which the spec leaves unbounded.
    expect(checked.length).toBeGreaterThan(0)
  })

  it("carries the API's list bounds on every taxonomy field", () => {
    const checked: string[] = []
    for (const [kind, [file, request]] of KINDS) {
      for (const spec of formFields(kind)) {
        if (spec.control !== 'taxonomy') continue
        const limits = requestLimits(file, request, spec.key)
        checked.push(`${kind}.${spec.key}`)
        expect(spec.maxItems).toBe(limits.maxItems)
        expect(spec.maxLength).toBe(limits.itemMaxLength)
      }
    }
    expect(checked.length).toBeGreaterThan(0)
  })

  it('reads the prompt name cap that differs from every other kind', () => {
    expect(
      requestLimits('prompts.yaml', 'CreatePromptRequest', 'name').maxLength
    ).toBe(50)
    const name = formFields('prompt').find(spec => spec.key === 'name')
    expect(name?.maxLength).toBe(50)
  })

  it('throws on a property the request schema does not declare', () => {
    expect(() =>
      requestLimits('prompts.yaml', 'CreatePromptRequest', 'nonesuch')
    ).toThrow(/CreatePromptRequest.nonesuch not found/)
  })
})
