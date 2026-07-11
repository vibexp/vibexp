import { type A2APart, isTextPart } from './a2a'

describe('isTextPart', () => {
  it('detects A2A v1.0 text parts (no kind field)', () => {
    const part = { text: 'hello' } as unknown as A2APart
    expect(isTextPart(part)).toBe(true)
  })

  it('detects legacy v0.x text parts (kind: text)', () => {
    const part = { kind: 'text', text: 'hi' } as A2APart
    expect(isTextPart(part)).toBe(true)
  })

  it('rejects non-text parts', () => {
    expect(isTextPart({ kind: 'image', source: 'x' })).toBe(false)
    expect(isTextPart({ kind: 'code', language: 'go', content: 'x' })).toBe(
      false
    )
  })
})
