/** Client mirror of #1147's preset rules (#1148). */
import { MAX_NAME, sameQuery, validatePresetName } from '../presetValidation'

const EXISTING = [
  { id: 'p1', name: 'Dormant teams' },
  { id: 'p2', name: 'Power users' },
]

describe('validatePresetName', () => {
  it('accepts a new, trimmed name', () => {
    expect(validatePresetName('  Big projects  ', EXISTING)).toBeNull()
  })

  it('requires a name after trimming', () => {
    expect(validatePresetName('   ', EXISTING)).toBe('Name is required')
  })

  it('caps the name at 80 characters, counting code points', () => {
    expect(validatePresetName('a'.repeat(MAX_NAME), EXISTING)).toBeNull()
    expect(validatePresetName('a'.repeat(MAX_NAME + 1), EXISTING)).toBe(
      'Name must be 80 characters or fewer'
    )
    // 80 emoji are 160 UTF-16 units but 80 runes, which the server accepts.
    expect(validatePresetName('😀'.repeat(MAX_NAME), EXISTING)).toBeNull()
  })

  it('rejects a duplicate name case-insensitively', () => {
    expect(validatePresetName('dormant TEAMS', EXISTING)).toBe(
      'A preset with this name already exists'
    )
  })

  it('lets a preset keep or re-case its own name', () => {
    expect(validatePresetName('DORMANT teams', EXISTING, 'p1')).toBeNull()
    expect(validatePresetName('Power users', EXISTING, 'p1')).toBe(
      'A preset with this name already exists'
    )
  })
})

describe('sameQuery', () => {
  it('ignores key order', () => {
    expect(sameQuery({ a: '1', b: '2' }, { b: '2', a: '1' })).toBe(true)
  })

  it('differs on a value, a missing key or an extra key', () => {
    expect(sameQuery({ a: '1' }, { a: '2' })).toBe(false)
    expect(sameQuery({ a: '1', b: '2' }, { a: '1' })).toBe(false)
    expect(sameQuery({ a: '1' }, { a: '1', b: '2' })).toBe(false)
    expect(sameQuery({ a: '1' }, { b: '1' })).toBe(false)
  })

  it('treats two empty queries as equal', () => {
    expect(sameQuery({}, {})).toBe(true)
  })
})
