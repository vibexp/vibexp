import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import {
  AUDIT_OPERATION_GROUPS,
  AUDIT_OPERATIONS,
  isAuditOperation,
  operationLabel,
} from '../operationLabels'

/**
 * The `AdminAuditOperation` union as the installed `@vibexp/api-client`
 * publishes it. The enum is type-only (no runtime value ships), so the
 * declaration file is the only runtime-readable copy of the vocabulary.
 */
function publishedOperations(): string[] {
  const schema = readFileSync(
    resolve(
      __dirname,
      '../../../../../node_modules/@vibexp/api-client/dist/schema.d.ts'
    ),
    'utf8'
  )
  const union = /AdminAuditOperation: ([^;]+);/.exec(schema)?.[1]
  if (union === undefined) {
    throw new Error('AdminAuditOperation is missing from the api-client schema')
  }
  return [...union.matchAll(/"([^"]+)"/g)].map(match => match[1])
}

describe('audit operation labels', () => {
  it('labels exactly the operations the published api-client enum lists', () => {
    const published = publishedOperations()
    expect(published.length).toBeGreaterThan(0)
    // Both directions: an enum value without a label, or a label for a value
    // the API no longer has, fails here.
    expect(Object.keys(AUDIT_OPERATIONS).sort()).toEqual([...published].sort())
  })

  it('gives every operation a distinct, non-empty label', () => {
    const labels = Object.values(AUDIT_OPERATIONS).map(info => info.label)
    expect(labels.every(label => label.trim() !== '')).toBe(true)
    expect(new Set(labels).size).toBe(labels.length)
  })

  it('puts every operation in exactly one listed group, and no group is empty', () => {
    const groupIds = AUDIT_OPERATION_GROUPS.map(group => group.id)
    const used = new Set(
      Object.values(AUDIT_OPERATIONS).map(info => info.group)
    )
    expect([...used].every(group => groupIds.includes(group))).toBe(true)
    expect(groupIds.every(group => used.has(group))).toBe(true)
  })

  it('words the refinement examples as written', () => {
    expect(operationLabel('prompt.update')).toBe('Prompt updated')
    expect(operationLabel('auth.login_failed')).toBe('Sign-in failed')
    expect(operationLabel('authz.denied')).toBe('Permission denied')
  })

  it('falls back to the dotted name for an operation newer than this build', () => {
    expect(isAuditOperation('widget.exploded')).toBe(false)
    expect(operationLabel('widget.exploded')).toBe('widget.exploded')
    expect(isAuditOperation('audit.export')).toBe(true)
  })
})
