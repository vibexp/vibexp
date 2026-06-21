import aiGenericIcon from '@/assets/ai-icons/ai-generic.svg'
import claudeIcon from '@/assets/ai-icons/claude.png'
import geminiIcon from '@/assets/ai-icons/gemini.png'
import openaiIcon from '@/assets/ai-icons/openai.webp'

export type AiProvider = 'claude' | 'openai' | 'gemini' | 'generic'

interface AiAssistantIcon {
  src: string
  provider: AiProvider
  alt: string
}

interface ProviderRule {
  provider: Exclude<AiProvider, 'generic'>
  tokens: string[]
  src: string
  alt: string
}

const PROVIDER_RULES: readonly ProviderRule[] = [
  {
    provider: 'claude',
    tokens: ['claude'],
    src: claudeIcon,
    alt: 'Claude logo',
  },
  {
    provider: 'openai',
    tokens: ['openai', 'codex'],
    src: openaiIcon,
    alt: 'OpenAI logo',
  },
  {
    provider: 'gemini',
    tokens: ['gemini', 'google'],
    src: geminiIcon,
    alt: 'Gemini logo',
  },
]

export function resolveAiAssistantIcon(
  assistantName: string | null | undefined
): AiAssistantIcon {
  const normalized = (assistantName ?? '').toLowerCase()
  for (const rule of PROVIDER_RULES) {
    if (rule.tokens.some(token => normalized.includes(token))) {
      return { src: rule.src, provider: rule.provider, alt: rule.alt }
    }
  }
  return {
    src: aiGenericIcon,
    provider: 'generic',
    alt: 'AI assistant logo',
  }
}
