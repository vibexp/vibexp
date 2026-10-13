import {
  buildOutline,
  createJumpPin,
  createSlugger,
  type OutlineHeading,
  pickActiveHeading,
  slugifyHeading,
} from '../outline'

describe('slugifyHeading', () => {
  it.each([
    ['Options considered', 'options-considered'],
    ['  What we deliberately did NOT do  ', 'what-we-deliberately-did-not-do'],
    ['Appendix: query set', 'appendix-query-set'],
    ['p95 latency (ms)', 'p95-latency-ms'],
    ['Übersicht — Ziele', 'übersicht-ziele'],
    ['???', ''],
  ])('%j → %j', (text, slug) => {
    expect(slugifyHeading(text)).toBe(slug)
  })
})

describe('createSlugger', () => {
  it('gives a repeated heading a numbered anchor, in document order', () => {
    const slug = createSlugger()

    expect([slug('Example'), slug('Example'), slug('Example')]).toEqual([
      'example',
      'example-2',
      'example-3',
    ])
  })

  it('never hands out an anchor twice, even when a heading is literally "Example 2"', () => {
    const slug = createSlugger()
    const anchors = [
      slug('Example'),
      slug('Example 2'),
      slug('Example'),
      slug('Example'),
    ]

    expect(new Set(anchors).size).toBe(anchors.length)
    expect(anchors).toEqual(['example', 'example-2', 'example-3', 'example-4'])
  })

  it('falls back to "section" for a heading with nothing sluggable', () => {
    const slug = createSlugger()

    expect([slug('???'), slug('!!!')]).toEqual(['section', 'section-2'])
  })

  it('starts fresh for each document', () => {
    expect(createSlugger()('Intro')).toBe('intro')
    expect(createSlugger()('Intro')).toBe('intro')
  })
})

describe('buildOutline', () => {
  const h = (level: number, id: string): OutlineHeading => ({
    id,
    level,
    text: id,
  })

  it('lists H2 and H3, and leaves H4 out when there are enough of those', () => {
    const outline = buildOutline([
      h(2, 'a'),
      h(3, 'a1'),
      h(4, 'deep'),
      h(2, 'b'),
      h(2, 'c'),
    ])

    expect(outline.map(e => e.id)).toEqual(['a', 'a1', 'b', 'c'])
  })

  it('lets H4 in when there are fewer than four H2/H3 entries', () => {
    const outline = buildOutline([h(2, 'a'), h(4, 'a-deep'), h(2, 'b')])

    expect(outline.map(e => e.id)).toEqual(['a', 'a-deep', 'b'])
  })

  it('is empty for a body with fewer than three headings', () => {
    expect(buildOutline([h(2, 'a'), h(2, 'b')])).toEqual([])
    expect(buildOutline([])).toEqual([])
  })

  it('never lists the H1', () => {
    const outline = buildOutline([
      h(1, 'title'),
      h(2, 'a'),
      h(2, 'b'),
      h(2, 'c'),
    ])

    expect(outline.map(e => e.id)).toEqual(['a', 'b', 'c'])
  })
})

describe('pickActiveHeading', () => {
  const tops = (...values: number[]) =>
    values.map((top, i) => ({ id: `h${String(i)}`, top }))

  it('is the last heading at or above the reading line', () => {
    expect(pickActiveHeading(tops(-400, 100, 130, 600), 130)).toBe('h2')
  })

  it('is the first heading before any has reached the line', () => {
    expect(pickActiveHeading(tops(300, 700), 130)).toBe('h0')
  })

  it('is the last heading once everything has scrolled past', () => {
    expect(pickActiveHeading(tops(-900, -500, -20), 130)).toBe('h2')
  })

  it('is null with no headings', () => {
    expect(pickActiveHeading([], 130)).toBeNull()
  })
})

describe('createJumpPin', () => {
  it('holds nothing until there is a jump', () => {
    const pin = createJumpPin()

    expect(pin.current()).toBeNull()
    expect(pin.scrolled()).toBe(false)
    expect(pin.userScrolled()).toBe(false)
  })

  it('pins the target of a jump', () => {
    const pin = createJumpPin()
    pin.jump('rollout')

    expect(pin.current()).toBe('rollout')
  })

  it('ignores the scrolling of the jump itself', () => {
    const pin = createJumpPin()
    pin.jump('rollout')

    // The smooth scroll that performs the jump fires many scroll events.
    expect(pin.scrolled()).toBe(false)
    expect(pin.scrolled()).toBe(false)
    expect(pin.current()).toBe('rollout')
  })

  it('keeps the pin when the jump settles, and releases it on the next scroll', () => {
    const pin = createJumpPin()
    pin.jump('appendix')
    pin.settle()
    expect(pin.current()).toBe('appendix')

    // Any scroll now is the reader's: a wheel, a dragged scrollbar, scrollTo.
    expect(pin.scrolled()).toBe(true)
    expect(pin.current()).toBeNull()
    // …and it is released once, not on every later scroll.
    expect(pin.scrolled()).toBe(false)
  })

  it('releases mid-jump on deliberate input: the reader has taken over', () => {
    const pin = createJumpPin()
    pin.jump('appendix')

    expect(pin.userScrolled()).toBe(true)
    expect(pin.current()).toBeNull()
    // The interrupted jump's remaining scroll events change nothing.
    expect(pin.scrolled()).toBe(false)
  })

  it('moves to a new target on a second jump, travelling again', () => {
    const pin = createJumpPin()
    pin.jump('summary')
    pin.settle()
    pin.jump('risks')

    expect(pin.current()).toBe('risks')
    expect(pin.scrolled()).toBe(false)
    pin.settle()
    expect(pin.scrolled()).toBe(true)
  })

  it('settling with nothing pinned is harmless', () => {
    const pin = createJumpPin()
    pin.settle()

    expect(pin.current()).toBeNull()
  })
})
