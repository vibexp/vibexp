import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { queryComponent, refTarget, systemStatuses } from './specYaml'

/**
 * The spec readers must be LOUD when they find nothing (see `specYaml.ts`): a
 * silently empty answer turns every assertion built on them vacuous. The happy
 * paths run against the real spec; each way of finding nothing runs against a
 * throwaway fixture, since the real spec is — by design — never in that state.
 */
describe('specYaml', () => {
  let dir: string
  const write = (name: string, body: string): string => {
    const path = join(dir, name)
    writeFileSync(path, body)
    return path
  }

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), 'spec-yaml-'))
  })

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true })
  })

  describe('refTarget', () => {
    it('follows a $ref across files to the component that defines it', () => {
      const paths = write('paths.yaml', '')
      write('root.yaml', "Status:\n  $ref: './schemas.yaml#/RealStatus'\n")
      const schemas = write('schemas.yaml', 'RealStatus:\n  type: string\n')

      expect(refTarget(paths, './root.yaml#/components/Status')).toEqual({
        file: schemas,
        name: 'RealStatus',
      })
    })

    it('throws when the component is not in the file it points at', () => {
      const paths = write('paths.yaml', '')
      write('root.yaml', 'Other:\n  type: string\n')

      expect(() => refTarget(paths, './root.yaml#/Status')).toThrow(
        /'Status' not found/
      )
    })

    it('throws instead of looping on a $ref that never resolves', () => {
      const paths = write('paths.yaml', '')
      write('root.yaml', "Status:\n  $ref: './root.yaml#/Status'\n")

      expect(() => refTarget(paths, './root.yaml#/Status')).toThrow(
        /'Status' never resolves/
      )
    })
  })

  describe('queryComponent', () => {
    it("resolves a list endpoint's status parameter to its schema", () => {
      const component = queryComponent('prompts.yaml', 'status')

      expect(component.name).toBe('PromptStatus')
      expect(component.file.endsWith(join('schemas', 'common.yaml'))).toBe(true)
    })

    it('throws when the path file has no such parameter', () => {
      expect(() => queryComponent('prompts.yaml', 'no_such_param')).toThrow(
        /'no_such_param' resolves to 0 schema components/
      )
    })

    it('throws when the parameter is declared inline', () => {
      const paths = write(
        'paths.yaml',
        'parameters:\n  - name: status\n    schema:\n      type: string\n'
      )

      expect(() => queryComponent(paths, 'status')).toThrow(
        /'status' has no \$ref/
      )
    })

    it('throws when two variants of the parameter disagree', () => {
      write('schemas.yaml', 'A:\n  type: string\nB:\n  type: string\n')
      const paths = write(
        'paths.yaml',
        [
          'parameters:',
          '  - name: status',
          "    schema:\n      $ref: './schemas.yaml#/A'",
          '  - name: status',
          "    schema:\n      $ref: './schemas.yaml#/B'",
          '',
        ].join('\n')
      )

      expect(() => queryComponent(paths, 'status')).toThrow(
        /'status' resolves to 2 schema components/
      )
    })
  })

  describe('systemStatuses', () => {
    const VOCABULARY = 'ResourceStatus:\n  enum: [active, draft, archived]\n'

    it('reads the system values a status schema names', () => {
      expect(systemStatuses(queryComponent('prompts.yaml', 'status'))).toEqual([
        'draft',
        'published',
      ])
    })

    it('returns only the vocabulary values the description names', () => {
      const file = write(
        'schemas.yaml',
        VOCABULARY +
          'Status:\n  description: >-\n    The system values are `active` and `draft`.\n'
      )

      expect(systemStatuses({ file, name: 'Status' })).toEqual([
        'active',
        'draft',
      ])
    })

    it('throws when the file has no ResourceStatus vocabulary', () => {
      const file = write('schemas.yaml', 'Status:\n  type: string\n')

      expect(() => systemStatuses({ file, name: 'Status' })).toThrow(
        /'ResourceStatus' declares no enum/
      )
    })

    it('throws when the schema is missing', () => {
      const file = write('schemas.yaml', VOCABULARY)

      expect(() => systemStatuses({ file, name: 'Status' })).toThrow(
        /'Status' not found/
      )
    })

    it('throws when the schema has grown an enum again', () => {
      const file = write(
        'schemas.yaml',
        VOCABULARY + 'Status:\n  enum: [active]\n'
      )

      expect(() => systemStatuses({ file, name: 'Status' })).toThrow(
        /'Status' declares an enum/
      )
    })

    it('throws when the description names no system value', () => {
      const file = write(
        'schemas.yaml',
        VOCABULARY + 'Status:\n  description: An open string.\n'
      )

      expect(() => systemStatuses({ file, name: 'Status' })).toThrow(
        /'Status' names no system status/
      )
    })
  })
})
