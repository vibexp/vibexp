import {
  buildSplitRows,
  buildUnifiedRows,
  computeDiffStat,
  hunkHeader,
} from '../diff'

describe('computeDiffStat', () => {
  it('counts added and removed lines', () => {
    // 'b' changed to 'B' (−1 +1) and 'd' added (+1)
    expect(computeDiffStat('a\nb\nc', 'a\nB\nc\nd')).toEqual({
      added: 2,
      removed: 1,
    })
  })

  it('returns zeros for identical content', () => {
    expect(computeDiffStat('same\nlines', 'same\nlines')).toEqual({
      added: 0,
      removed: 0,
    })
  })

  it('handles empty content', () => {
    expect(computeDiffStat('', 'one\ntwo')).toEqual({ added: 2, removed: 0 })
    expect(computeDiffStat('one\ntwo', '')).toEqual({ added: 0, removed: 2 })
  })
})

describe('buildSplitRows', () => {
  it('aligns context, modified and added lines side by side', () => {
    const rows = buildSplitRows('a\nb', 'a\nB\nc')
    expect(rows.map(r => r.kind)).toEqual(['context', 'mod', 'add'])

    // context row: same text both sides, both line numbers
    expect(rows[0].left.num).toBe(1)
    expect(rows[0].right.num).toBe(1)

    // modified row: word-level highlight on the changed token
    expect(rows[1].left.num).toBe(2)
    expect(rows[1].right.num).toBe(2)
    expect(
      rows[1].left.segments?.some(s => s.changed && s.text.includes('b'))
    ).toBe(true)
    expect(
      rows[1].right.segments?.some(s => s.changed && s.text.includes('B'))
    ).toBe(true)

    // added row: left is a striped empty cell, right has the new line
    expect(rows[2].left.segments).toBeNull()
    expect(rows[2].right.num).toBe(3)
  })

  it('renders pure deletions with an empty right cell', () => {
    const rows = buildSplitRows('a\nb\nc', 'a\nc')
    const del = rows.find(r => r.kind === 'del')
    expect(del).toBeDefined()
    expect(del?.right.segments).toBeNull()
  })
})

describe('buildUnifiedRows', () => {
  it('emits context, removed and added rows with correct gutters', () => {
    const rows = buildUnifiedRows('a\nb', 'a\nB')
    expect(rows.map(r => r.kind)).toEqual(['context', 'del', 'add'])
    const del = rows[1]
    expect(del.leftNum).toBe(2)
    expect(del.rightNum).toBeNull()
    const add = rows[2]
    expect(add.leftNum).toBeNull()
    expect(add.rightNum).toBe(2)
  })
})

describe('hunkHeader', () => {
  it('summarises both sides line spans', () => {
    expect(hunkHeader('a\nb', 'a\nB\nc')).toBe('@@ -1,2 +1,3 @@')
  })
})
