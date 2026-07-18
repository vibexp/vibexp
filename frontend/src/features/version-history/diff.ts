import { diffLines, diffWords } from 'diff'

// Diff engine for the version-history compare view. Produces aligned split rows
// (old | new side-by-side), unified rows, and a +N/−M line diffstat. Resource-
// agnostic: it operates on raw old/new text, so any versioned resource reuses it.

export interface DiffStat {
  added: number
  removed: number
}

// A run of text within a line, flagged when it differs from the other side
// (drives the word-level highlight in modified rows).
export interface WordSegment {
  text: string
  changed: boolean
}

// `null` segments mark a cell that has no line on that side — rendered as the
// diagonally-striped "empty" cell in the design.
export interface SplitCell {
  num: number | null
  segments: WordSegment[] | null
}

export type SplitKind = 'context' | 'add' | 'del' | 'mod'

export interface SplitRow {
  kind: SplitKind
  left: SplitCell
  right: SplitCell
}

export type UnifiedKind = 'context' | 'add' | 'del'

export interface UnifiedRow {
  kind: UnifiedKind
  leftNum: number | null
  rightNum: number | null
  text: string
}

type BlockType = 'context' | 'add' | 'del'
interface Block {
  type: BlockType
  lines: string[]
}

// Split a diff chunk value into lines. The `diff` library keeps the trailing
// newline on each value, so "a\nb\n" and "a\nb" both yield ["a", "b"]; "" yields
// [] (no spurious empty line).
function toLines(value: string): string[] {
  const lines = value.split('\n')
  if (lines.length > 0 && lines.at(-1) === '') {
    lines.pop()
  }
  return lines
}

// Ensure a trailing newline so the final line isn't treated as different from a
// middle line (diffLines otherwise reports a phantom change on the last line when
// only one side has a trailing newline).
function normalize(text: string): string {
  if (text.length === 0) return ''
  return text.endsWith('\n') ? text : `${text}\n`
}

function linesDiff(oldText: string, newText: string) {
  return diffLines(normalize(oldText), normalize(newText))
}

function blockType(part: { added?: boolean; removed?: boolean }): BlockType {
  if (part.added) return 'add'
  if (part.removed) return 'del'
  return 'context'
}

function toBlocks(oldText: string, newText: string): Block[] {
  return linesDiff(oldText, newText).map(part => ({
    type: blockType(part),
    lines: toLines(part.value),
  }))
}

// +N/−M line counts between two texts.
export function computeDiffStat(oldText: string, newText: string): DiffStat {
  let added = 0
  let removed = 0
  for (const part of linesDiff(oldText, newText)) {
    const count = toLines(part.value).length
    if (part.added) added += count
    else if (part.removed) removed += count
  }
  return { added, removed }
}

// Word-level diff of two single lines → [leftSegments, rightSegments]. The left
// cell keeps removed + common runs; the right keeps added + common runs.
function wordSegments(
  oldLine: string,
  newLine: string
): [WordSegment[], WordSegment[]] {
  const left: WordSegment[] = []
  const right: WordSegment[] = []
  for (const part of diffWords(oldLine, newLine)) {
    if (part.added) {
      right.push({ text: part.value, changed: true })
    } else if (part.removed) {
      left.push({ text: part.value, changed: true })
    } else {
      left.push({ text: part.value, changed: false })
      right.push({ text: part.value, changed: false })
    }
  }
  return [left, right]
}

const plain = (text: string): WordSegment[] => [{ text, changed: false }]

// Mutable left/right line counters shared by the split-row emitters below.
interface LineNums {
  left: number
  right: number
}

function pushContextRows(rows: SplitRow[], nums: LineNums, lines: string[]) {
  for (const line of lines) {
    rows.push({
      kind: 'context',
      left: { num: nums.left++, segments: plain(line) },
      right: { num: nums.right++, segments: plain(line) },
    })
  }
}

function pushDelRows(rows: SplitRow[], nums: LineNums, lines: string[]) {
  for (const line of lines) {
    rows.push({
      kind: 'del',
      left: { num: nums.left++, segments: plain(line) },
      right: { num: null, segments: null },
    })
  }
}

function pushAddRows(rows: SplitRow[], nums: LineNums, lines: string[]) {
  for (const line of lines) {
    rows.push({
      kind: 'add',
      left: { num: null, segments: null },
      right: { num: nums.right++, segments: plain(line) },
    })
  }
}

// A removed run immediately followed by an added run: pair lines
// index-for-index (word-diffed 'mod' rows), leftover lines fall back to pure
// del/add rows with a striped empty counterpart.
function pushPairedRows(
  rows: SplitRow[],
  nums: LineNums,
  dels: string[],
  adds: string[]
) {
  const pairs = Math.max(dels.length, adds.length)
  for (let k = 0; k < pairs; k++) {
    const hasDel = k < dels.length
    const hasAdd = k < adds.length
    if (hasDel && hasAdd) {
      const [ls, rs] = wordSegments(dels[k], adds[k])
      rows.push({
        kind: 'mod',
        left: { num: nums.left++, segments: ls },
        right: { num: nums.right++, segments: rs },
      })
    } else if (hasDel) {
      pushDelRows(rows, nums, [dels[k]])
    } else {
      pushAddRows(rows, nums, [adds[k]])
    }
  }
}

// Aligned side-by-side rows. A removed run immediately followed by an added run
// is treated as a modification: lines are paired index-for-index (word-diffed),
// leftover lines fall back to pure del/add rows with a striped empty counterpart.
export function buildSplitRows(oldText: string, newText: string): SplitRow[] {
  const blocks = toBlocks(oldText, newText)
  const rows: SplitRow[] = []
  const nums: LineNums = { left: 1, right: 1 }

  for (let i = 0; i < blocks.length; i++) {
    const block = blocks[i]

    if (block.type === 'context') {
      pushContextRows(rows, nums, block.lines)
    } else if (block.type === 'del') {
      const hasPairedAdd = i + 1 < blocks.length && blocks[i + 1].type === 'add'
      if (hasPairedAdd) {
        pushPairedRows(rows, nums, block.lines, blocks[i + 1].lines)
        i++ // consumed the paired add block
      } else {
        pushDelRows(rows, nums, block.lines)
      }
    } else {
      // pure add (not preceded by a del)
      pushAddRows(rows, nums, block.lines)
    }
  }

  return rows
}

// Single-column unified rows: context, removed (left gutter), added (right gutter).
export function buildUnifiedRows(
  oldText: string,
  newText: string
): UnifiedRow[] {
  const blocks = toBlocks(oldText, newText)
  const rows: UnifiedRow[] = []
  let leftNum = 1
  let rightNum = 1

  for (const block of blocks) {
    for (const line of block.lines) {
      if (block.type === 'context') {
        rows.push({
          kind: 'context',
          leftNum: leftNum++,
          rightNum: rightNum++,
          text: line,
        })
      } else if (block.type === 'del') {
        rows.push({
          kind: 'del',
          leftNum: leftNum++,
          rightNum: null,
          text: line,
        })
      } else {
        rows.push({
          kind: 'add',
          leftNum: null,
          rightNum: rightNum++,
          text: line,
        })
      }
    }
  }

  return rows
}

// git-style hunk header summarising the two sides' line spans.
export function hunkHeader(oldText: string, newText: string): string {
  const oldCount = toLines(oldText).length
  const newCount = toLines(newText).length
  return `@@ -1,${String(oldCount)} +1,${String(newCount)} @@`
}
