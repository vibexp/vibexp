import { substituteFilledPlaceholders } from './markdownPlaceholders'

describe('substituteFilledPlaceholders', () => {
  const values = { name: 'Ada', empty: '', 'two words': 'both' }

  it('substitutes the placeholders that have a value', () => {
    expect(substituteFilledPlaceholders('Hi {{name}}!', values)).toBe('Hi Ada!')
    expect(substituteFilledPlaceholders('{{ name }}{{name}}', values)).toBe(
      'AdaAda'
    )
    expect(substituteFilledPlaceholders('{{two words}}', values)).toBe('both')
  })

  it('leaves a placeholder with no value as written', () => {
    expect(substituteFilledPlaceholders('{{missing}} {{empty}}', values)).toBe(
      '{{missing}} {{empty}}'
    )
  })

  it('does not treat unbalanced or empty braces as a placeholder', () => {
    for (const text of ['{{}}', '{{name}', '{name}}', '{{name', 'a }} b']) {
      expect(substituteFilledPlaceholders(text, values)).toBe(text)
    }
  })

  it('reads the key from the first pair of braces that opens it', () => {
    // As the pattern it replaces did: the key is everything up to the first
    // closing brace, an extra opening brace included.
    expect(substituteFilledPlaceholders('{{{name}}', { '{name': 'X' })).toBe(
      'X'
    )
    expect(substituteFilledPlaceholders('{{a}b}} {{name}}', values)).toBe(
      '{{a}b}} Ada'
    )
  })

  it('stays fast on a long run of opening braces', () => {
    const text = '{'.repeat(50_000)
    const started = performance.now()
    expect(substituteFilledPlaceholders(text, values)).toBe(text)
    expect(performance.now() - started).toBeLessThan(1000)
  })
})
