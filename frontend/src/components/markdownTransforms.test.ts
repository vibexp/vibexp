import {
  addHeadingIds,
  markTaskListItems,
  replaceCodeBlocks,
  replaceMermaidFences,
  stripLeadingTitleHeading,
} from './markdownTransforms'

describe('stripLeadingTitleHeading', () => {
  const BODY = '# Search ranking redesign\n\nSearch is the entry point.\n'

  it('drops a leading H1 that repeats the title', () => {
    expect(stripLeadingTitleHeading(BODY, 'Search ranking redesign')).toBe(
      '\nSearch is the entry point.\n'
    )
  })

  it.each([
    ['case', 'SEARCH RANKING redesign'],
    ['surrounding whitespace', '  Search ranking redesign  '],
    ['inner whitespace runs', 'Search  ranking   redesign'],
  ])('ignores %s when matching', (_what, title) => {
    expect(stripLeadingTitleHeading(BODY, title)).not.toContain('# Search')
  })

  it.each([
    ['blank lines before it', '\n\n# Title\n\nBody'],
    ['up to three spaces of indent', '   # Title\n\nBody'],
    ['a closing hash run', '# Title ##\n\nBody'],
    ['inline markers around the words', '# **Title**\n\nBody'],
    ['a setext underline', 'Title\n=====\n\nBody'],
    ['CRLF line endings', '# Title\r\n\r\nBody'],
  ])('handles %s', (_what, source) => {
    const result = stripLeadingTitleHeading(source, 'Title')

    expect(result).not.toMatch(/Title/)
    expect(result).toMatch(/Body$/)
  })

  it.each([
    ['a different H1', '# Something else\n\nBody'],
    ['an H2 with the same words', '## Title\n\nBody'],
    ['the same H1 further down', 'Intro\n\n# Title\n\nBody'],
    ['a paragraph with the same words', 'Title\n\nBody'],
    ['a hash that is not a heading', '#Title\n\nBody'],
    ['a heading that only starts with the title', '# Title and more\n\nBody'],
  ])('keeps %s', (_what, source) => {
    expect(stripLeadingTitleHeading(source, 'Title')).toBe(source)
  })

  it.each([[undefined], [''], ['   ']])(
    'changes nothing for a title of %j',
    title => {
      expect(stripLeadingTitleHeading(BODY, title)).toBe(BODY)
    }
  )

  it('changes nothing for an empty body', () => {
    expect(stripLeadingTitleHeading('', 'Title')).toBe('')
    expect(stripLeadingTitleHeading('\n\n', 'Title')).toBe('\n\n')
  })
})

describe('markTaskListItems', () => {
  it('marks an unchecked item and replaces its checkbox', () => {
    const html = markTaskListItems(
      '<ul>\n<li><input disabled="" type="checkbox"> Draft the plan</li>\n</ul>'
    )

    expect(html).toContain('<li class="task-list-item">')
    expect(html).toContain('data-checked="false"')
    expect(html).toContain('aria-label="Not completed"')
    expect(html).toContain('Draft the plan')
    // Read-only by construction: no input is left to toggle.
    expect(html).not.toContain('<input')
  })

  it('marks a checked item as completed', () => {
    const html = markTaskListItems(
      '<li><input checked="" disabled="" type="checkbox"> Ship it</li>'
    )

    expect(html).toContain('data-checked="true"')
    expect(html).toContain('aria-label="Completed"')
  })

  it('handles a loose list, where the checkbox sits inside a paragraph', () => {
    const html = markTaskListItems(
      '<li><p><input disabled="" type="checkbox"> Loose item</p>\n</li>'
    )

    expect(html).toMatch(
      /^<li class="task-list-item"><p><span class="task-checkbox"/
    )
  })

  it('leaves ordinary list items alone', () => {
    const html = '<ul><li>Plain</li><li><strong>Bold</strong> item</li></ul>'

    expect(markTaskListItems(html)).toBe(html)
  })

  it('leaves an input that is not a checkbox alone', () => {
    const html = '<li><input type="text" value="x"></li>'

    expect(markTaskListItems(html)).toBe(html)
  })

  it('marks each item of a mixed list independently', () => {
    const html = markTaskListItems(
      '<li><input checked="" disabled="" type="checkbox"> a</li><li><input disabled="" type="checkbox"> b</li><li>c</li>'
    )

    expect(html.match(/task-list-item/g)).toHaveLength(2)
    expect(html.match(/data-checked="true"/g)).toHaveLength(1)
    expect(html).toContain('<li>c</li>')
  })
})

describe('addHeadingIds', () => {
  it('gives each heading an id derived from its text', () => {
    expect(
      addHeadingIds('<h2>Options considered</h2><p>x</p><h3>Rollout plan</h3>')
    ).toBe(
      '<h2 id="options-considered">Options considered</h2><p>x</p><h3 id="rollout-plan">Rollout plan</h3>'
    )
  })

  it('de-duplicates repeated headings within the document', () => {
    const html = addHeadingIds(
      '<h2>Example</h2><h2>Example</h2><h3>Example</h3>'
    )

    expect(html).toContain('<h2 id="example">')
    expect(html).toContain('<h2 id="example-2">')
    expect(html).toContain('<h3 id="example-3">')
  })

  it('slugs the text, not the markup or the entities inside a heading', () => {
    expect(
      addHeadingIds('<h2>Use <code>a &lt; b</code> &amp; <em>more</em></h2>')
    ).toContain('<h2 id="use-a-b-more">')
  })

  it('leaves a heading that already has attributes alone', () => {
    const html = '<h2 id="mine">Kept</h2>'

    expect(addHeadingIds(html)).toBe(html)
  })

  it('numbers each document on its own', () => {
    expect(addHeadingIds('<h2>Intro</h2>')).toBe(
      addHeadingIds('<h2>Intro</h2>')
    )
  })
})

describe('addHeadingIds — headings that are not closed', () => {
  it('leaves an unclosed heading alone and still marks the ones after it', () => {
    expect(addHeadingIds('<h2>Open <h3>Closed</h3>')).toBe(
      '<h2>Open <h3 id="closed">Closed</h3>'
    )
  })

  it('closes a heading at the first matching closing tag', () => {
    expect(addHeadingIds('<h2>A <h3>B</h2></h3>')).toBe(
      '<h2 id="a-b">A <h3>B</h2></h3>'
    )
  })

  it('stays fast on a long run of opening tags that never close', () => {
    const html = '<h1>'.repeat(50_000)
    const started = performance.now()
    expect(addHeadingIds(html)).toBe(html)
    expect(performance.now() - started).toBeLessThan(1000)
  })
})

describe('addHeadingIds — markup inside a heading', () => {
  it('derives the id from the text, without the tags', () => {
    expect(addHeadingIds('<h2>The <em>fast</em> <code>path</code></h2>')).toBe(
      '<h2 id="the-fast-path">The <em>fast</em> <code>path</code></h2>'
    )
  })

  it('stays fast on a heading that is a long run of angle brackets', () => {
    const inner = '&lt;'.repeat(10) + '<'.repeat(50_000)
    const started = performance.now()
    expect(addHeadingIds(`<h2>${inner}</h2>`)).toContain(`${inner}</h2>`)
    expect(performance.now() - started).toBeLessThan(1000)
  })
})

describe('replaceCodeBlocks', () => {
  const mark = (className: string | undefined, code: string) =>
    `[${String(className)}|${code}]`

  it('hands over the class and the body of each block', () => {
    expect(
      replaceCodeBlocks(
        '<p>a</p><pre><code class="language-ts">x &lt; y</code></pre><pre><code>plain</code></pre>',
        mark
      )
    ).toBe('<p>a</p>[language-ts|x &lt; y][undefined|plain]')
  })

  it('ends a block at the first closing pair', () => {
    expect(
      replaceCodeBlocks('<pre><code>a</code></pre>b</code></pre>', mark)
    ).toBe('[undefined|a]b</code></pre>')
  })

  it('leaves what is not exactly a block marked emits', () => {
    for (const html of [
      '<pre><code>never closed',
      '<pre><code id="x">a</code></pre>',
      '<pre><code class="a" id="x">a</code></pre>',
      '<pre><code class="unterminated>a</code></pre>',
      '<pre>a</pre>',
    ]) {
      expect(replaceCodeBlocks(html, mark)).toBe(html)
    }
  })

  it('stays fast on a long run of openings that never close', () => {
    for (const html of [
      '<pre><code>'.repeat(50_000),
      '<pre><code class="'.repeat(50_000),
    ]) {
      const started = performance.now()
      expect(replaceCodeBlocks(html, mark)).toBe(html)
      expect(performance.now() - started).toBeLessThan(1000)
    }
  })
})

describe('replaceMermaidFences', () => {
  const F = '```'
  const mark = (code: string) => `[${code}]`

  it('hands over the body of a mermaid fence, and of an mmd one', () => {
    expect(
      replaceMermaidFences(`a\n${F}mermaid\ngraph TD\n${F}\nb`, mark)
    ).toBe('a\n[graph TD\n]\nb')
    expect(replaceMermaidFences(`${F}mmd\nA-->B\n${F}`, mark)).toBe('[A-->B\n]')
  })

  it('accepts CRLF after the info string', () => {
    expect(replaceMermaidFences(`${F}mermaid\r\ngraph TD\r\n${F}`, mark)).toBe(
      '[graph TD\r\n]'
    )
  })

  it('replaces each diagram in a body with several', () => {
    expect(
      replaceMermaidFences(
        `${F}mermaid\none\n${F}\ntext\n${F}mmd\ntwo\n${F}`,
        mark
      )
    ).toBe('[one\n]\ntext\n[two\n]')
  })

  it('leaves a fence that is never closed', () => {
    const source = `intro\n${F}mermaid\ngraph TD`
    expect(replaceMermaidFences(source, mark)).toBe(source)
  })

  it('leaves other fences, and an info string with anything after it', () => {
    for (const source of [
      `${F}ts\nconst a = 1\n${F}`,
      `${F}mermaid title\ngraph TD\n${F}`,
      `${F}mermaidx\ngraph TD\n${F}`,
      `${F}mermaid`,
    ]) {
      expect(replaceMermaidFences(source, mark)).toBe(source)
    }
  })

  it('stays fast on long adversarial input', () => {
    for (const source of [
      // One opening, then a long body with backtick pairs and no closing.
      `${F}mermaid\n${'`` mermaid\n'.repeat(5_000)}`,
      // Nothing but backticks: every position starts a fence of no kind.
      '`'.repeat(50_000),
      // Openings back to back: each one's backticks close the one before.
      `${F}mermaid\n`.repeat(5_000),
    ]) {
      const started = performance.now()
      replaceMermaidFences(source, mark)
      expect(performance.now() - started).toBeLessThan(1000)
    }
  })
})
