import { forwardFinder, trimChar, trimEndChar } from '../linearText'

describe('trimEndChar', () => {
  it('drops only the trailing run', () => {
    expect(trimEndChar('/a//b///', '/')).toBe('/a//b')
    expect(trimEndChar('abc', '/')).toBe('abc')
    expect(trimEndChar('///', '/')).toBe('')
    expect(trimEndChar('', '/')).toBe('')
  })
})

describe('trimChar', () => {
  it('drops the leading and trailing runs and keeps what is between', () => {
    expect(trimChar('--a--b--', '-')).toBe('a--b')
    expect(trimChar('a', '-')).toBe('a')
    expect(trimChar('----', '-')).toBe('')
    expect(trimChar('', '-')).toBe('')
  })
})

describe('forwardFinder', () => {
  it('answers like indexOf for positions that only move forward', () => {
    const text = 'a]bc]d'
    const find = forwardFinder(text, ']')
    expect(find(0)).toBe(1)
    expect(find(1)).toBe(1)
    expect(find(2)).toBe(4)
    expect(find(4)).toBe(4)
    expect(find(5)).toBe(-1)
    expect(find(6)).toBe(-1)
  })

  it('searches the text once when the needle is absent', () => {
    const text = 'x'.repeat(1000)
    const spy = vi.spyOn(String.prototype, 'indexOf')
    const find = forwardFinder(text, ']')
    for (let from = 0; from < text.length; from += 1) find(from)
    expect(spy).toHaveBeenCalledTimes(1)
    spy.mockRestore()
  })
})

/** Runs `work` and fails if it takes longer than a linear scan ever would. */
function expectFast(work: () => void): void {
  const started = performance.now()
  work()
  expect(performance.now() - started).toBeLessThan(1000)
}

describe('on a long adversarial input', () => {
  const N = 50_000

  it('trims in linear time when the run is not at the end', () => {
    expectFast(() => {
      expect(trimEndChar(`${'/'.repeat(N)}a`, '/')).toHaveLength(N + 1)
      expect(trimChar(`a${'-'.repeat(N)}a`, '-')).toHaveLength(N + 2)
    })
  })
})
