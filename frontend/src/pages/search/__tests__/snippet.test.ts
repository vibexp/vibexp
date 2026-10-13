import { highlightSegments, queryTerms, stripMarkdown } from '../snippet'

describe('stripMarkdown', () => {
  it('drops heading markers and keeps the heading text', () => {
    expect(stripMarkdown('## Deploy steps\nRun the script')).toBe(
      'Deploy steps Run the script'
    )
  })

  it('drops emphasis markers', () => {
    expect(
      stripMarkdown('a **bold** and *soft* and __loud__ and ~~gone~~')
    ).toBe('a bold and soft and loud and gone')
  })

  it('leaves snake_case identifiers alone', () => {
    expect(stripMarkdown('set team_id and _then_ run')).toBe(
      'set team_id and then run'
    )
  })

  it('drops code fences and backticks, keeping the code', () => {
    expect(stripMarkdown('Run:\n```bash\nmake test\n```\nor `make lint`')).toBe(
      'Run: make test or make lint'
    )
  })

  it('keeps link and image text, dropping the target', () => {
    expect(
      stripMarkdown('See [the guide](https://example.com/a) and ![logo](x.png)')
    ).toBe('See the guide and logo')
  })

  it('drops list, task and quote markers', () => {
    expect(stripMarkdown('- one\n* two\n1. three\n- [x] done\n> quoted')).toBe(
      'one two three done quoted'
    )
  })

  it('flattens a table', () => {
    expect(
      stripMarkdown('| Name | Role |\n| --- | --- |\n| Ada | Admin |')
    ).toBe('Name Role Ada Admin')
  })

  it('cleans up the half of a pair a slice left behind', () => {
    // An excerpt is cut out of the body, so its markup is often unbalanced.
    expect(stripMarkdown('ends the **bold part\n```ts\nconst a = 1')).toBe(
      'ends the bold part const a = 1'
    )
  })

  it('returns plain text unchanged', () => {
    expect(stripMarkdown('Nothing to strip here.')).toBe(
      'Nothing to strip here.'
    )
  })
})

describe('queryTerms', () => {
  it('splits on whitespace, lowercases and drops search punctuation', () => {
    expect(queryTerms('"Deploy" -checklist  OR')).toEqual([
      'checklist',
      'deploy',
      'or',
    ])
  })

  it('ignores single characters and repeats', () => {
    expect(queryTerms('a go Go')).toEqual(['go'])
  })
})

describe('highlightSegments', () => {
  it('flags every case-insensitive occurrence of a term', () => {
    expect(highlightSegments('Deploy the deploy script', 'deploy')).toEqual([
      { text: 'Deploy', match: true },
      { text: ' the ', match: false },
      { text: 'deploy', match: true },
      { text: ' script', match: false },
    ])
  })

  it('highlights each term of a multi-word query', () => {
    const matched = highlightSegments('review the code', 'code review')
      .filter(segment => segment.match)
      .map(segment => segment.text)
    expect(matched).toEqual(['review', 'code'])
  })

  it('prefers the longer of two overlapping terms', () => {
    const matched = highlightSegments('testing', 'test testing')
      .filter(segment => segment.match)
      .map(segment => segment.text)
    expect(matched).toEqual(['testing'])
  })

  it('treats query characters literally, not as a pattern', () => {
    const matched = highlightSegments('axb or a.b', 'a.b')
      .filter(segment => segment.match)
      .map(segment => segment.text)
    expect(matched).toEqual(['a.b'])
  })

  it('returns the text whole when there is nothing to highlight', () => {
    expect(highlightSegments('plain text', '')).toEqual([
      { text: 'plain text', match: false },
    ])
    expect(highlightSegments('', 'term')).toEqual([])
  })
})
