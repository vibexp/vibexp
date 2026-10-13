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

describe('stripMarkdown — links and tables, edge cases', () => {
  it('unwraps inline and reference links and images', () => {
    expect(stripMarkdown('See [the docs](https://x.test) now')).toBe(
      'See the docs now'
    )
    expect(stripMarkdown('![alt text](a.png) and [ref][1]')).toBe(
      'alt text and ref'
    )
  })

  it('leaves brackets that are not a link', () => {
    expect(stripMarkdown('[not a link] (really)')).toBe('[not a link] (really)')
    expect(stripMarkdown('a [half](open')).toBe('a [half](open')
  })

  it('blanks a table divider row but not a row of text', () => {
    expect(stripMarkdown('| A | B |\n|---|:--|\n| 1 | 2 |')).toBe('A B 1 2')
    expect(stripMarkdown('a - b')).toBe('a - b')
  })
})

describe('on a long adversarial input', () => {
  const N = 50_000
  const fast = (work: () => void) => {
    const started = performance.now()
    work()
    expect(performance.now() - started).toBeLessThan(1000)
  }

  it('unwraps links in linear time on a run of opening brackets', () => {
    fast(() => {
      expect(stripMarkdown('['.repeat(N))).toBe('['.repeat(N))
    })
  })

  it('handles brackets that close but are never a link', () => {
    fast(() => {
      expect(stripMarkdown(`${'['.repeat(N)}]x`)).toBe(`${'['.repeat(N)}]x`)
    })
  })

  it('handles reference-style openings with no closing bracket', () => {
    fast(() => {
      const text = `[a]${'['.repeat(N)}`
      expect(stripMarkdown(text)).toBe(text)
    })
  })

  it('checks a long line of dashes and pipes once', () => {
    fast(() => {
      expect(stripMarkdown(`${'-|'.repeat(N)}x`)).toContain('x')
    })
  })

  it('trims a query term wrapped in a long run of punctuation', () => {
    fast(() => {
      expect(queryTerms(`${'"'.repeat(N)}term${'"'.repeat(N)}`)).toEqual([
        'term',
      ])
      expect(queryTerms(`ab${'-'.repeat(N)}cd`)).toEqual([
        `ab${'-'.repeat(N)}cd`,
      ])
    })
  })
})

describe('stripMarkdown — emphasis and table pipes, edge cases', () => {
  it('unwraps strong, underscore-strong and strikethrough', () => {
    expect(stripMarkdown('a **bold** b __also__ c ~~gone~~ d')).toBe(
      'a bold b also c gone d'
    )
  })

  it('closes at the nearest marker that follows a non-space', () => {
    // The middle `**` follows a space, so it cannot close; the last one does.
    // (A `**` left over would be dropped afterwards; `__` is not, so it shows.)
    expect(stripMarkdown('__a __ b__ c')).toBe('a __ b c')
    expect(stripMarkdown('**a ** b** c')).toBe('a b c')
  })

  it('does not pair markers across a line break', () => {
    // The unpaired markers are then dropped as leftovers, each on its line.
    expect(stripMarkdown('**a\nb**')).toBe('a b')
    expect(stripMarkdown('__a\nb__')).toBe('__a b__')
  })

  it('leaves a marker that opens onto a space', () => {
    expect(stripMarkdown('a __ b __ c')).toBe('a __ b __ c')
  })

  it('turns each pipe and the blanks around it into one space', () => {
    expect(stripMarkdown('a | b')).toBe('a b')
    expect(stripMarkdown('a|b||c')).toBe('a b c')
    expect(stripMarkdown('| cell |')).toBe('cell')
  })
})

describe('emphasis and pipes on a long adversarial input', () => {
  const N = 50_000
  const fast = (work: () => void) => {
    const started = performance.now()
    work()
    expect(performance.now() - started).toBeLessThan(1000)
  }

  it('handles opening markers that never close, on one line', () => {
    fast(() => {
      // Every `__` is followed by a letter and preceded by a space, so each
      // could open and none can close.
      const text = '__a '.repeat(N / 4)
      expect(stripMarkdown(text)).toBe(text.trim())
    })
  })

  it('handles strikethrough openings that never close', () => {
    fast(() => {
      expect(stripMarkdown('~~a '.repeat(N / 4))).toBe(
        'a '.repeat(N / 4).trim()
      )
    })
  })

  it('handles a long run of blanks with no pipe after it', () => {
    fast(() => {
      expect(stripMarkdown(`a${' '.repeat(N)}b`)).toBe('a b')
      expect(stripMarkdown(`a${'\t '.repeat(N / 2)}|b`)).toBe('a b')
    })
  })
})
