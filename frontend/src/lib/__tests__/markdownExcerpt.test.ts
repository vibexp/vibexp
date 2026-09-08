import { contentLines, markdownToExcerpt } from '../markdownExcerpt'

describe('markdownToExcerpt', () => {
  it('strips heading markers', () => {
    expect(
      markdownToExcerpt('## Decision D3\n\nRepositories are tenancy-only.', 140)
    ).toBe('Decision D3 Repositories are tenancy-only.')
  })

  it('strips list markers at any level and both bullet styles', () => {
    expect(
      markdownToExcerpt('- one\n+ two\n* three\n1. four\n2) five', 140)
    ).toBe('one two three four five')
  })

  it('strips block quotes', () => {
    expect(markdownToExcerpt('> quoted line\n>tight quote', 140)).toBe(
      'quoted line tight quote'
    )
  })

  it('strips emphasis and inline-code delimiters', () => {
    expect(
      markdownToExcerpt('**Never** use `--no-verify` or ~~that~~', 140)
    ).toBe('Never use --no-verify or that')
  })

  it('keeps underscores — snake_case is far likelier than markdown stress', () => {
    expect(markdownToExcerpt('use snake_case for db columns', 140)).toBe(
      'use snake_case for db columns'
    )
  })

  it('drops the contents of a fenced code block', () => {
    const text = [
      '```bash',
      '# install deps',
      'npm ci',
      '```',
      'Real body',
    ].join('\n')
    expect(markdownToExcerpt(text, 140)).toBe('Real body')
  })

  it('drops a tilde-fenced block too', () => {
    expect(
      markdownToExcerpt('~~~sh\n# not a heading\n~~~\nReal body', 140)
    ).toBe('Real body')
  })

  it('does not let a shorter fence close a longer one', () => {
    // CommonMark: the closing run must be at least as long as the opening one.
    expect(markdownToExcerpt('````\n```\nhidden\n````\nReal body', 140)).toBe(
      'Real body'
    )
  })

  it('handles a CRLF body', () => {
    expect(markdownToExcerpt('# Title\r\n**bold** body', 140)).toBe(
      'Title bold body'
    )
  })

  it('handles a CR-only body', () => {
    expect(markdownToExcerpt('# Title\r**bold** body', 140)).toBe(
      'Title bold body'
    )
  })

  it('still skips code fences in a CR-only body', () => {
    expect(markdownToExcerpt('```\r# secret\r```\rReal body', 140)).toBe(
      'Real body'
    )
  })

  it('returns an empty string for an empty body — never a bare ellipsis', () => {
    expect(markdownToExcerpt('', 140)).toBe('')
  })

  it('returns an empty string for a whitespace-only body', () => {
    expect(markdownToExcerpt('   \n\t  \n ', 140)).toBe('')
  })

  it('returns an empty string when the only content is a code fence', () => {
    expect(markdownToExcerpt('```\nsome code\n```', 140)).toBe('')
  })

  it('leaves a body shorter than the cap untouched, with no ellipsis', () => {
    expect(markdownToExcerpt('short body', 140)).toBe('short body')
  })

  it('keeps a body exactly at the cap intact', () => {
    const exact = 'y'.repeat(140)
    expect(markdownToExcerpt(exact, 140)).toBe(exact)
  })

  it('breaks on a whole word and appends an ellipsis when truncated', () => {
    const excerpt = markdownToExcerpt(
      'The quick brown fox jumps over the lazy dog and then keeps running for miles',
      40
    )
    expect(excerpt).toBe('The quick brown fox jumps over the lazy…')
    expect(excerpt.length).toBeLessThanOrEqual(41)
  })

  it('hard-cuts a single word longer than the cap', () => {
    expect(markdownToExcerpt('x'.repeat(60), 20)).toBe(`${'x'.repeat(20)}…`)
  })

  it('collapses newlines and runs of whitespace into single spaces', () => {
    expect(markdownToExcerpt('one\n\n  two   three', 140)).toBe('one two three')
  })

  it('stays linear on a large CR-only body', () => {
    // A `\r` the split does not cover makes the whole body one line, which took
    // ~8.5s at 200 KB before the terminator set was widened (#902).
    const started = performance.now()
    markdownToExcerpt(`${' '.repeat(100_000)}\r${'x'.repeat(100_000)}\ry`, 140)
    expect(performance.now() - started).toBeLessThan(500)
  })
})

describe('contentLines', () => {
  it('returns the body lines with fenced blocks removed', () => {
    expect(contentLines('a\n```\nhidden\n```\nb')).toEqual(['a', 'b'])
  })

  it('splits on every CommonMark line terminator', () => {
    expect(contentLines('a\r\nb\rc\nd\u2028e\u2029f')).toEqual([
      'a',
      'b',
      'c',
      'd',
      'e',
      'f',
    ])
  })
})
