import { deriveMemoryTitle } from '../memoryTitle'

describe('deriveMemoryTitle', () => {
  it('uses the first markdown heading when there is one', () => {
    expect(
      deriveMemoryTitle('# Deployment runbook\n\nAlways drain the node first.')
    ).toBe('Deployment runbook')
  })

  it('accepts any ATX level, leading indent and closing hashes', () => {
    expect(deriveMemoryTitle('   ### Cache keys ###\nbody')).toBe('Cache keys')
  })

  it('takes the FIRST heading, not a later one', () => {
    expect(deriveMemoryTitle('intro line\n# First\n## Second')).toBe('First')
  })

  it('ignores a "#" that is inside a fenced code block', () => {
    const text = [
      '```bash',
      '# install deps',
      'npm ci',
      '```',
      '# Real title',
    ].join('\n')
    expect(deriveMemoryTitle(text)).toBe('Real title')
  })

  it('does not let a shorter fence close a longer one', () => {
    // CommonMark: the closing run must be at least as long as the opening one,
    // so the `# x` here is still inside the ```` block.
    const text = ['````', '```', '# x', '````', '# Real title'].join('\n')
    expect(deriveMemoryTitle(text)).toBe('Real title')
  })

  it('finds the heading in a CRLF body', () => {
    expect(deriveMemoryTitle('# Title\r\nbody')).toBe('Title')
  })

  it('finds the heading in a CR-only body', () => {
    expect(deriveMemoryTitle('# Title\rbody line here')).toBe('Title')
  })

  it('still skips code fences in a CR-only body', () => {
    expect(deriveMemoryTitle('```\r# secret\r```\rReal body')).toBe('Real body')
  })

  it('stays linear on a CR-only body with no heading', () => {
    // Before the split covered `\r`, the whole body reached ATX_HEADING as one
    // line and this took ~8.5s at 200 KB.
    const started = performance.now()
    deriveMemoryTitle(`# ${' '.repeat(100_000)}\r${'x'.repeat(100_000)}\ry`)
    expect(performance.now() - started).toBeLessThan(500)
  })

  it('ignores a tilde-fenced block too, and falls back to the prose', () => {
    const text = ['~~~sh', '# not a heading', '~~~', 'Plain body line'].join(
      '\n'
    )
    expect(deriveMemoryTitle(text)).toBe('Plain body line')
  })

  it('falls back to the plain-text body when there is no heading', () => {
    expect(deriveMemoryTitle('Remember to rotate the signing key.')).toBe(
      'Remember to rotate the signing key.'
    )
  })

  it('strips list markers, quotes and emphasis from the excerpt', () => {
    expect(deriveMemoryTitle('- **Never** use `--no-verify`')).toBe(
      'Never use --no-verify'
    )
  })

  it('keeps underscores — snake_case is far likelier than markdown stress', () => {
    expect(deriveMemoryTitle('use snake_case for db columns')).toBe(
      'use snake_case for db columns'
    )
  })

  it('collapses newlines and runs of whitespace into single spaces', () => {
    expect(deriveMemoryTitle('one\n\n  two   three')).toBe('one two three')
  })

  it('truncates a long body on a word boundary with an ellipsis', () => {
    const title = deriveMemoryTitle(
      'The quick brown fox jumps over the lazy dog and then keeps running for miles'
    )
    expect(title).toBe(
      'The quick brown fox jumps over the lazy dog and then keeps…'
    )
    // 60 characters of body at most, plus the ellipsis — and never a half word.
    expect(title.length).toBeLessThanOrEqual(61)
    expect(title.endsWith('s…')).toBe(true)
  })

  it('hard-cuts a single word longer than the limit', () => {
    const title = deriveMemoryTitle('x'.repeat(120))
    expect(title).toBe(`${'x'.repeat(60)}…`)
  })

  it('keeps a body exactly at the limit intact', () => {
    const exact = 'y'.repeat(60)
    expect(deriveMemoryTitle(exact)).toBe(exact)
  })

  it('keeps a closing hash that is not preceded by whitespace', () => {
    expect(deriveMemoryTitle('# The C# playbook')).toBe('The C# playbook')
  })

  it('stays linear when whitespace precedes a long hash run', () => {
    // The `/\\s+#+\\s*$/` trim this replaced took ~1.4s on this input.
    const started = performance.now()
    deriveMemoryTitle(`# a${'#'.repeat(40_000)}${' '.repeat(40_000)}x`)
    expect(performance.now() - started).toBeLessThan(500)
  })

  it('stays linear on a pathological heading (no catastrophic backtracking)', () => {
    // The lazy `(.+?)\\s*#*\\s*$` shape this replaced took ~3.2s at n=40k.
    const started = performance.now()
    expect(deriveMemoryTitle(`# ${'#'.repeat(40_000)}a`)).toContain('#')
    expect(performance.now() - started).toBeLessThan(500)
  })

  it('returns the fallback for an empty body', () => {
    expect(deriveMemoryTitle('')).toBe('Untitled memory')
  })

  it('returns the fallback for a whitespace-only body', () => {
    expect(deriveMemoryTitle('   \n\t  \n ')).toBe('Untitled memory')
  })

  it('returns the fallback when the only content is a code fence', () => {
    expect(deriveMemoryTitle('```\nsome code\n```')).toBe('Untitled memory')
  })

  it('skips an empty heading and uses the body instead', () => {
    expect(deriveMemoryTitle('#\nActual content')).toBe('Actual content')
  })
})
