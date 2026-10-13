import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'

import { buildOutline, type OutlineHeading, pickActiveHeading } from './outline'

/**
 * How far below the top of the viewport the "reading line" sits: just under
 * the app header and the sticky title bar. It is the headings'
 * `scroll-margin-top` (7.5rem = 120px, index.css) plus a few pixels, so the
 * heading a reader jumps to — which lands exactly on the scroll margin — is at
 * or above the line and is the one marked current.
 */
const READING_LINE_PX = 128

/** What a reader does to scroll by hand, as opposed to a jump we started. */
const MANUAL_SCROLL_EVENTS = ['wheel', 'touchmove', 'keydown'] as const

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
 * line — plus one `scrollend` per scroll to settle jumps; never with a
 * per-frame scroll listener.
 */
export function useReadingOutline(
  articleRef: RefObject<HTMLElement | null>,
  enabled: boolean
): ReadingOutline {
  const [entries, setEntries] = useState<OutlineHeading[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  // The entry the reader just jumped to. It is the current section, full
  // stop, until they scroll by hand: positions read while the smooth scroll is
  // still travelling would mark whatever it is passing, and near the end of a
  // document the page cannot scroll far enough for a late heading to reach
  // the reading line at all.
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
      if (jumpedToRef.current !== null) {
        setActiveId(jumpedToRef.current)
        return
      }
      setActiveId(
        pickActiveHeading(
          elements.map(el => ({
            id: el.id,
            top: el.getBoundingClientRect().top,
          })),
          READING_LINE_PX
        )
      )
    }
    // Scrolling by hand hands the outline back to position. These fire per
    // gesture, not per frame, and only clear a flag — the observer below is
    // still what recomputes.
    const release = () => {
      jumpedToRef.current = null
    }
    MANUAL_SCROLL_EVENTS.forEach(type => {
      globalThis.addEventListener(type, release, { passive: true })
    })

    update()
    // Everything from the reading line down to the bottom of the viewport: a
    // heading enters or leaves that region exactly when it crosses the line
    // (the current section changes) or the bottom edge, so the observer fires
    // while the reader scrolls without a scroll listener.
    const observer = new IntersectionObserver(update, {
      rootMargin: `-${String(READING_LINE_PX)}px 0px 0px 0px`,
    })
    // A jump (or a fling across a section taller than the viewport) can move
    // headings past the line without any of them ever being inside that
    // region at an observed frame. `scrollend` fires once per scroll, not per
    // frame, and settles the answer.
    globalThis.addEventListener('scrollend', update, { passive: true })
    elements.forEach(el => {
      observer.observe(el)
    })
    return () => {
      observer.disconnect()
      globalThis.removeEventListener('scrollend', update)
      MANUAL_SCROLL_EVENTS.forEach(type => {
        globalThis.removeEventListener(type, release)
      })
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
