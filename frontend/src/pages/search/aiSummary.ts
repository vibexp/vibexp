import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { buildResourceUrl } from '@/lib/resourceUrl'
import type {
  SearchFilterType,
  SearchSummaryResponse,
  SearchSummarySource,
} from '@/services/searchService'
import { ApiError } from '@/types/errors'

/**
 * One search's AI Summary, held by the search page (never inside the
 * collapsible's subtree — Radix unmounts a closed panel's children, which
 * would destroy it and re-fire the request on every expand).
 */
export type SummaryState =
  | { status: 'loading' }
  | { status: 'ready'; data: SearchSummaryResponse }
  | { status: 'error'; code: string; message: string }

/**
 * Identity of a search as far as its summary is concerned. The page is
 * deliberately NOT part of it: the summary is grounded in the global top-N
 * results, so paging must reuse it rather than regenerate.
 */
export function summaryKey(
  teamId: string,
  query: string,
  type: SearchFilterType | undefined,
  projectId: string | undefined
): string {
  return JSON.stringify([
    teamId,
    query.trim().toLowerCase(),
    type ?? '',
    projectId ?? '',
  ])
}

const ERROR_MESSAGES = new Map<string, string>(
  Object.entries({
    AI_SUMMARY_DISABLED: 'AI Summary is turned off for this team.',
    AI_SUMMARY_NO_PROVIDER:
      'No model provider is configured for this team, so a summary cannot be generated.',
    AI_SUMMARY_NO_RESULTS:
      'There are no matching documents to base a summary on.',
    AI_SUMMARY_PROVIDER_UNREACHABLE:
      'The model provider could not be reached. Try again in a moment.',
    AI_SUMMARY_UNAUTHORIZED:
      'The model provider rejected its credentials. Check the provider’s API key in team settings.',
    AI_SUMMARY_MODEL_ERROR:
      'The model returned an error while generating the summary.',
    AI_SUMMARY_TIMEOUT: 'The model took too long to respond.',
    NETWORK_ERROR: 'Could not reach the server. Check your connection.',
    CLIENT_TIMEOUT: 'The request timed out before a summary arrived.',
  })
)

const UNKNOWN_ERROR_MESSAGE = 'Something went wrong generating the summary.'

/**
 * Map a failed summary request to a stable code and a user-facing message.
 * `unwrap` surfaces problem details as `ApiError` (whose `code` is one of the
 * `AI_SUMMARY_*` codes) and transport failures as plain `Error`s.
 */
export function classifySummaryError(error: unknown): {
  code: string
  message: string
} {
  let code = 'UNKNOWN'
  if (error instanceof ApiError) {
    code = error.code
  } else if (error instanceof Error) {
    if (error.message.startsWith('Network error')) code = 'NETWORK_ERROR'
    else if (error.message.startsWith('Request timeout'))
      code = 'CLIENT_TIMEOUT'
  }
  return { code, message: ERROR_MESSAGES.get(code) ?? UNKNOWN_ERROR_MESSAGE }
}

/** The in-app URL of a summary source, or `null` when it cannot be built. */
export function sourceUrl(source: SearchSummarySource): string | null {
  return buildResourceUrl({
    type: source.type,
    id: source.id,
    slug: source.slug,
    projectId: source.project_id,
  })
}

const CITATION = /\[(\d+)\]/g

/** Elements whose text is never rewritten into citation links. */
const NO_CITATION_ANCESTORS = 'a, code, pre'

/**
 * Replace every `[n]` in `textNode` that names a known source with a citation
 * link. Unknown numbers stay literal text.
 */
function linkCitations(
  textNode: Text,
  sourcesByIndex: Map<number, SearchSummarySource>
): void {
  const text = textNode.data
  const doc = textNode.ownerDocument
  const fragment = doc.createDocumentFragment()
  let last = 0
  for (const match of text.matchAll(CITATION)) {
    const index = Number(match[1])
    const source = sourcesByIndex.get(index)
    const url = source ? sourceUrl(source) : null
    if (!source || !url) continue
    fragment.append(text.slice(last, match.index))
    const link = doc.createElement('a')
    link.setAttribute('href', url)
    link.dataset.citation = String(index)
    link.dataset.resourceId = source.id
    link.className = 'ai-summary-citation'
    link.textContent = `[${String(index)}]`
    fragment.append(link)
    last = match.index + match[0].length
  }
  if (last === 0) return
  fragment.append(text.slice(last))
  textNode.replaceWith(fragment)
}

/**
 * Render a summary's markdown to HTML safe for `dangerouslySetInnerHTML`.
 *
 * The summary is untrusted LLM output: markdown → HTML (`marked`), then `[n]`
 * citations are rewritten into links to their sources in an inert parsed
 * document (DOMParser runs no scripts and loads nothing), and only THEN is
 * the whole thing sanitized — sanitizing last means nothing the rewrite did
 * can re-open a hole.
 */
export function renderSummaryHtml(
  summary: string,
  sources: readonly SearchSummarySource[]
): string {
  const html = marked.parse(summary, { async: false })
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const sourcesByIndex = new Map(sources.map(s => [s.index, s]))

  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT)
  const textNodes: Text[] = []
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (!node.parentElement?.closest(NO_CITATION_ANCESTORS)) {
      textNodes.push(node as Text)
    }
  }
  for (const node of textNodes) linkCitations(node, sourcesByIndex)

  return DOMPurify.sanitize(doc.body.innerHTML)
}
