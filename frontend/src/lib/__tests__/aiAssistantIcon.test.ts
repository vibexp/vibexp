import { resolveAiAssistantIcon } from '../aiAssistantIcon'

describe('resolveAiAssistantIcon', () => {
  it.each([
    ['claude', 'claude'],
    ['Claude', 'claude'],
    ['claude-sonnet-4-7', 'claude'],
    ['Anthropic Claude', 'claude'],
    ['CLAUDE-OPUS', 'claude'],
  ])('matches %s to claude provider', (name, expected) => {
    const result = resolveAiAssistantIcon(name)
    expect(result.provider).toBe(expected)
    expect(result.src).toBeTruthy()
  })

  it.each([
    ['openai', 'openai'],
    ['OpenAI GPT-4', 'openai'],
    ['codex', 'openai'],
    ['Codex CLI', 'openai'],
    ['CODEX', 'openai'],
  ])('matches %s to openai provider', (name, expected) => {
    const result = resolveAiAssistantIcon(name)
    expect(result.provider).toBe(expected)
  })

  it.each([
    ['gemini', 'gemini'],
    ['Gemini Pro', 'gemini'],
    ['google', 'gemini'],
    ['Google Bard', 'gemini'],
    ['GEMINI-1.5', 'gemini'],
  ])('matches %s to gemini provider', (name, expected) => {
    const result = resolveAiAssistantIcon(name)
    expect(result.provider).toBe(expected)
  })

  it.each(['mistral', 'llama', 'unknown-bot', 'GitHub Copilot', ''])(
    'falls back to generic for "%s"',
    name => {
      const result = resolveAiAssistantIcon(name)
      expect(result.provider).toBe('generic')
      expect(result.src).toBeTruthy()
    }
  )

  it.each([null, undefined])('falls back to generic when name is %p', value => {
    const result = resolveAiAssistantIcon(value)
    expect(result.provider).toBe('generic')
  })

  it('returns the same provider regardless of casing', () => {
    expect(resolveAiAssistantIcon('CLAUDE').provider).toBe(
      resolveAiAssistantIcon('claude').provider
    )
  })

  it('returns a non-empty alt text for accessibility', () => {
    expect(resolveAiAssistantIcon('claude').alt).toMatch(/claude/i)
    expect(resolveAiAssistantIcon('mistral').alt.length).toBeGreaterThan(0)
  })
})
