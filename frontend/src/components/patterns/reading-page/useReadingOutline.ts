import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'

import { buildOutline, type OutlineHeading, pickActiveHeading } from './outline'

/**
 * How far below the top of the viewport the "reading line" sits: under the app
 * header and the sticky title bar, matching the headings' `scroll-margin-top`.
 */
const READING_LINE_PX = 130

const HEADING_SELECTOR = '.reading-body :is(h2, h3, h4)[id]'

export interface ReadingOutline {
  entries: OutlineHeading[]
  /** The id of the section the reader is in, or `null` with no outline. */
  activeId: string | null
  /** Scrolls to a section and puts it in the URL hash, so it can be linked. */
  goTo: (id: string) => void
}

const sameEntries = (a: OutlineHeading[], b: OutlineHeading[]): boolean =>
  a.length === b.length &&
  a.every((h, i) => h.id === b[i].id && h.text === b[i].text)

/**
 * The outline of the rendered body inside `articleRef`: its entries, the
 * section in view, and how to jump to one.
 *
 * The body is rendered asynchronously and re-rendered when it changes (a
 * placeholder value, the Raw view), so the headings are read from the DOM and
 * re-read when it mutates. The section in view is tracked with an
 * IntersectionObserver on the headings — it fires as one crosses the reading
 * line — and never with a scroll listener.
 */
export function useReadingOutline(
  articleRef: RefObject<HTMLElement | null>,
  enabled: boolean
): ReadingOutline {
  const [entries, setEntries] = useState<OutlineHeading[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  // The entry the reader just jumped to. Near the end of a document the page
  // cannot scroll far enough for a late heading to reach the reading line, so
  // position alone would keep marking the section before it; the jump's target
  // stays current for as long as it is on screen below the line.
  const jumpedToRef = useRef<string | null>(null)

  // Collect the headings, now and whenever the body's DOM changes.
  useEffect(() => {
    const article = articleRef.current
    if (!enabled || !article) return

    const collect = () => {
      const headings = [
        ...article.querySelectorAll<HTMLElement>(HEADING_SELECTOR),
      ].map(el => ({
        id: el.id,
        level: Number(el.tagName.slice(1)),
        text: el.textContent.trim(),
      }))
      const next = buildOutline(headings)
      setEntries(current => (sameEntries(current, next) ? current : next))
    }

    collect()
    const observer = new MutationObserver(collect)
    observer.observe(article, { childList: true, subtree: true })
    return () => {
      observer.disconnect()
    }
  }, [articleRef, enabled])

  // Track the section in view.
  useEffect(() => {
    const article = articleRef.current
    if (!article || entries.length === 0) return
    if (typeof IntersectionObserver === 'undefined') return

    const elements = entries.flatMap(entry => {
      const el = document.getElementById(entry.id)
      return el && article.contains(el) ? [el] : []
    })
    const update = () => {
      const tops = elements.map(el => ({
        id: el.id,
        top: el.getBoundingClientRect().top,
      }))
      const jumped = tops.find(heading => heading.id === jumpedToRef.current)
      if (
        jumped &&
        jumped.top > READING_LINE_PX &&
        jumped.top < globalThis.innerHeight
      ) {
        setActiveId(jumped.id)
        return
      }
      jumpedToRef.current = null
      setActiveId(pickActiveHeading(tops, READING_LINE_PX))
    }

    update()
    // A thin band at the reading line: the observer fires when a heading
    // enters or leaves it, which is exactly when the current section changes.
    const observer = new IntersectionObserver(update, {
      rootMargin: `-${String(READING_LINE_PX - 10)}px 0px -70% 0px`,
      threshold: [0, 1],
    })
    elements.forEach(el => {
      observer.observe(el)
    })
    return () => {
      observer.disconnect()
    }
  }, [articleRef, entries])

  // A URL that arrives with a hash scrolls to its heading once the body has
  // rendered — the browser's own attempt ran before the heading existed.
  const [hashHandled, setHashHandled] = useState(false)
  useEffect(() => {
    if (hashHandled || entries.length === 0) return
    setHashHandled(true)
    const id = decodeURIComponent(globalThis.location.hash.slice(1))
    if (id && entries.some(entry => entry.id === id)) {
      jumpedToRef.current = id
      document.getElementById(id)?.scrollIntoView({ block: 'start' })
    }
  }, [entries, hashHandled])

  const goTo = useCallback((id: string) => {
    const target = document.getElementById(id)
    if (!target) return
    target.scrollIntoView({ block: 'start', behavior: 'smooth' })
    // replaceState, not a navigation: the section is linkable without adding
    // a history entry per click or re-running the router.
    globalThis.history.replaceState(
      globalThis.history.state,
      '',
      `#${encodeURIComponent(id)}`
    )
    jumpedToRef.current = id
    setActiveId(id)
  }, [])

  return {
    entries: enabled ? entries : [],
    activeId: entries.length > 0 ? activeId : null,
    goTo,
  }
}
