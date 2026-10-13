import { indentSelection } from '../indent'
import { changedSpan } from '../useRawEditorKeys'

/** `|` marks the caret; a pair marks a selection. */
function parse(marked: string) {
  const start = marked.indexOf('|')
  const rest = marked.slice(0, start) + marked.slice(start + 1)
  const second = rest.indexOf('|')
  if (second === -1) return { value: rest, start, end: start }
  return {
    value: rest.slice(0, second) + rest.slice(second + 1),
    start,
    end: second,
  }
}

function show({ value, start, end }: ReturnType<typeof parse>): string {
  if (start === end) return `${value.slice(0, start)}|${value.slice(start)}`
  return `${value.slice(0, start)}|${value.slice(start, end)}|${value.slice(end)}`
}

const indent = (marked: string) => show(indentSelection(parse(marked)))
const outdent = (marked: string) => show(indentSelection(parse(marked), true))

describe('indentSelection — indent', () => {
  it('indents the line the caret is on and carries the caret along', () => {
    expect(indent('one\ntw|o\nthree')).toBe('one\n  tw|o\nthree')
  })

  it('indents an empty single line, so a new list item can be nested', () => {
    expect(indent('one\n|')).toBe('one\n  |')
  })

  it('indents every line a selection touches', () => {
    expect(indent('a\nb|b\ncc|c\nd')).toBe('a\n  b|b\n  cc|c\nd')
  })

  it('leaves out the line a selection merely ends at the start of', () => {
    expect(indent('|one\ntwo\n|three')).toBe('  |one\n  two\n|three')
  })

  it('leaves blank lines inside a selection blank', () => {
    expect(indent('|a\n\nb|')).toBe('  |a\n\n  b|')
  })
})

describe('indentSelection — outdent', () => {
  it('removes one step from the current line', () => {
    expect(outdent('one\n    tw|o')).toBe('one\n  tw|o')
  })

  it('removes a single leading space when that is all there is', () => {
    expect(outdent(' on|e')).toBe('on|e')
  })

  it('removes a leading tab', () => {
    expect(outdent('\ton|e')).toBe('on|e')
  })

  it('never pulls the caret back onto the previous line', () => {
    expect(outdent('one\n |two')).toBe('one\n|two')
  })

  it('outdents each selected line by what it has', () => {
    expect(outdent('|  a\n b\nc|')).toBe('|a\nb\nc|')
  })

  it('changes nothing when there is nothing to remove', () => {
    const before = parse('on|e\ntwo')
    expect(indentSelection(before, true)).toEqual(before)
  })

  it('undoes an indent', () => {
    const before = parse('a\nb|b\ncc|c\nd')
    expect(indentSelection(indentSelection(before), true)).toEqual(before)
  })
})

describe('changedSpan', () => {
  it('finds the smallest replacement between two texts', () => {
    expect(changedSpan('one\ntwo\nthree', 'one\n  two\nthree')).toEqual({
      from: 4,
      to: 4,
      text: '  ',
    })
  })

  it('describes a removal as an empty replacement', () => {
    expect(changedSpan('a\n  b', 'a\nb')).toEqual({ from: 2, to: 4, text: '' })
  })
})
