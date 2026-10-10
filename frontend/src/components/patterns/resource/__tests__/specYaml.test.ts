import {
  namedStatuses,
  paramChunks,
  queryEnum,
  systemStatuses,
} from './specYaml'

/**
 * The spec readers are what the descriptor↔API assertions stand on, so the
 * ways they refuse to answer are pinned here: a reader that returned nothing
 * instead of throwing would turn every assertion built on it vacuous.
 */

const VOCABULARY = ['active', 'draft', 'archived', 'published', 'expired']

describe('paramChunks', () => {
  const yaml = [
    'parameters:',
    '  - name: status',
    '    in: query',
    '  - name: status_reason',
    '    in: query',
    '  - name: sort_by',
    '    schema:',
    '      enum: [name]',
    '  - name: status',
    '    in: path',
  ].join('\n')

  it('returns one chunk per declaration of the parameter', () => {
    expect(paramChunks(yaml, 'status')).toEqual([
      '    in: query',
      '    in: path',
    ])
  })

  it('does not match a parameter that merely starts with the name', () => {
    expect(paramChunks(yaml, 'status_reason')).toEqual(['    in: query'])
  })

  it('stops a chunk at the next parameter', () => {
    expect(paramChunks(yaml, 'sort_by')).toEqual([
      '    schema:\n      enum: [name]',
    ])
  })

  it('returns nothing for a parameter the file does not declare', () => {
    expect(paramChunks(yaml, 'page')).toEqual([])
  })
})

describe('namedStatuses', () => {
  it('returns the vocabulary members the description names in backticks', () => {
    const block = [
      '  type: string',
      '  description: >-',
      '    The system values are `active` and `expired`. Not archived.',
    ].join('\n')
    expect(namedStatuses(block, VOCABULARY)).toEqual(['active', 'expired'])
  })

  it('throws when the schema is an enum again', () => {
    const block = '  type: string\n  enum: [active, `draft`]'
    expect(() => namedStatuses(block, VOCABULARY)).toThrow(/declares an enum/)
  })

  it('throws when the description names no system status', () => {
    const block = '  type: string\n  description: Any status the team defines.'
    expect(() => namedStatuses(block, VOCABULARY)).toThrow(
      /names no system status/
    )
  })
})

describe('systemStatuses', () => {
  it("reads a resource type's system statuses out of the spec", () => {
    expect([...systemStatuses('blueprints.yaml')].sort()).toEqual([
      'active',
      'expired',
    ])
  })

  it('throws for a path file with no status parameter', () => {
    expect(() => systemStatuses('types.yaml')).toThrow(
      /declares no status parameter/
    )
  })
})

describe('queryEnum', () => {
  it('still reads a closed enum behind a $ref', () => {
    expect(queryEnum('blueprints.yaml', 'sort_by').size).toBeGreaterThan(0)
  })

  it('throws for a parameter whose schema is no longer an enum', () => {
    expect(() => queryEnum('blueprints.yaml', 'status')).toThrow(
      /declares no enum/
    )
  })
})
