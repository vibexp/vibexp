import {
  MAX_METADATA_KEY_LENGTH,
  MAX_METADATA_PAIRS,
  type MetadataPair,
  recombineMetadata,
  splitMetadata,
  validateMetadataRows,
} from '../metadataPairs'

describe('splitMetadata', () => {
  it('returns empty pairs and extras for undefined', () => {
    expect(splitMetadata(undefined)).toEqual({ pairs: [], extras: {} })
  })

  it('splits string entries into pairs and everything else into extras', () => {
    const { pairs, extras } = splitMetadata({
      author: 'ada',
      model: 'opus',
      tags: ['a', 'b'],
      count: 3,
      nested: { deep: true },
      flag: false,
      empty: null,
    })
    expect(pairs).toEqual([
      { key: 'author', value: 'ada' },
      { key: 'model', value: 'opus' },
    ])
    expect(extras).toEqual({
      tags: ['a', 'b'],
      count: 3,
      nested: { deep: true },
      flag: false,
      empty: null,
    })
  })

  it('preserves insertion order of string pairs', () => {
    const { pairs } = splitMetadata({ z: '1', a: '2', m: '3' })
    expect(pairs.map(p => p.key)).toEqual(['z', 'a', 'm'])
  })
})

describe('recombineMetadata', () => {
  it('merges trimmed string pairs with extras', () => {
    const result = recombineMetadata([{ key: ' author ', value: 'ada' }], {
      tags: ['x'],
    })
    expect(result).toEqual({ author: 'ada', tags: ['x'] })
  })

  it('drops pairs with a blank (whitespace-only) key', () => {
    const result = recombineMetadata(
      [
        { key: '   ', value: 'orphan' },
        { key: 'kept', value: 'v' },
      ],
      {}
    )
    expect(result).toEqual({ kept: 'v' })
  })

  it('never lets a colliding string row clobber a preserved non-string extra', () => {
    const result = recombineMetadata([{ key: 'tags', value: 'oops' }], {
      tags: ['real'],
    })
    expect(result).toEqual({ tags: ['real'] })
  })

  it('round-trips complex metadata untouched through split then recombine', () => {
    const original = {
      author: 'ada',
      tags: ['a', 'b'],
      config: { retries: 2, nested: { on: true } },
      count: 7,
      flag: false,
    }
    const { pairs, extras } = splitMetadata(original)
    expect(recombineMetadata(pairs, extras)).toEqual(original)
  })
})

const rows = (...entries: [string, string][]): MetadataPair[] =>
  entries.map(([key, value]) => ({ key, value }))

describe('validateMetadataRows', () => {
  it('accepts well-formed rows', () => {
    const result = validateMetadataRows(rows(['a', '1'], ['b', '2']))
    expect(result.valid).toBe(true)
    expect(result.rowErrors).toEqual([null, null])
    expect(result.formError).toBeNull()
  })

  it('flags a blank key', () => {
    const result = validateMetadataRows(rows(['  ', 'v']))
    expect(result.valid).toBe(false)
    expect(result.rowErrors[0]).toBe('Key is required')
  })

  it('flags a blank value', () => {
    const result = validateMetadataRows(rows(['k', '   ']))
    expect(result.valid).toBe(false)
    expect(result.rowErrors[0]).toBe('Value is required')
  })

  it('flags duplicate keys on every offending row', () => {
    const result = validateMetadataRows(
      rows(['dup', '1'], ['dup', '2'], ['ok', '3'])
    )
    expect(result.rowErrors[0]).toBe('Duplicate key')
    expect(result.rowErrors[1]).toBe('Duplicate key')
    expect(result.rowErrors[2]).toBeNull()
    expect(result.valid).toBe(false)
  })

  it('treats trimmed keys as duplicates', () => {
    const result = validateMetadataRows(rows(['dup', '1'], [' dup ', '2']))
    expect(result.rowErrors[0]).toBe('Duplicate key')
    expect(result.rowErrors[1]).toBe('Duplicate key')
  })

  it('flags an over-long key', () => {
    const longKey = 'k'.repeat(MAX_METADATA_KEY_LENGTH + 1)
    const result = validateMetadataRows(rows([longKey, 'v']))
    expect(result.rowErrors[0]).toContain(String(MAX_METADATA_KEY_LENGTH))
    expect(result.valid).toBe(false)
  })

  it('allows a key exactly at the length cap', () => {
    const maxKey = 'k'.repeat(MAX_METADATA_KEY_LENGTH)
    const result = validateMetadataRows(rows([maxKey, 'v']))
    expect(result.valid).toBe(true)
  })

  it('flags a reserved key', () => {
    const result = validateMetadataRows(rows(['tags', 'v']), {
      reservedKeys: ['tags'],
    })
    expect(result.rowErrors[0]).toContain('reserved')
    expect(result.valid).toBe(false)
  })

  it('does not treat a required key as reserved', () => {
    const result = validateMetadataRows(rows(['model', 'opus']), {
      reservedKeys: ['model'],
      requiredKeys: ['model'],
    })
    expect(result.rowErrors[0]).toBeNull()
    expect(result.valid).toBe(true)
  })

  it('flags a row that collides with a preserved extra key', () => {
    const result = validateMetadataRows(rows(['tags', 'v']), {
      extrasKeys: ['tags'],
    })
    expect(result.rowErrors[0]).toContain('conflicts')
    expect(result.valid).toBe(false)
  })

  it('flags exceeding the pair-count cap', () => {
    const many = Array.from(
      { length: MAX_METADATA_PAIRS + 1 },
      (_, i): [string, string] => [`k${String(i)}`, 'v']
    )
    const result = validateMetadataRows(rows(...many))
    expect(result.formError).toContain(String(MAX_METADATA_PAIRS))
    expect(result.valid).toBe(false)
  })
})
