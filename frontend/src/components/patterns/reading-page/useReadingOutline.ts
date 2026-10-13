import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'

import {
  buildOutline,
  createJumpPin,
  type OutlineHeading,
  pickActiveHeading,
} from './outline'

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

/**
 * How long a jump may travel before it is treated as settled anyway — for a
 * jump that moves nothing (the target is already in place) and so never fires
 * `scrollend`, and for browsers without that event.
 */
const JUMP_SETTLE_FALLBACK_MS = 1200

/** How long a jump is given to start moving the page before it is taken as a no-op. */
const JUMP_START_GRACE_MS = 150

/** How long after a hash load the body is given to finish laying out. */
const HASH_SETTLE_WINDOW_MS = 4000

const HEADING_SELECTOR = '.reading-body :is(h2, h3, h4)[id]'

const atDocumentEnd = (): boolean =>
  globalThis.innerHeight + globalThis.scrollY >=
  document.documentElement.scrollHeight - 2

const isOnScreen = (el: Element): boolean => {
  const { top, bottom } = el.getBoundingClientRect()
  return bottom > 0 && top < globalThis.innerHeight
}

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
 * line — plus one `scrollend` per scroll to settle the answer. The only
 * `scroll` listener does no measuring: it releases a jump's pin.
 */
export function useReadingOutline(
  articleRef: RefObject<HTMLElement | null>,
  enabled: boolean
): ReadingOutline {
  const [entries, setEntries] = useState<OutlineHeading[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  // The entry the reader just jumped to, until they scroll again themselves
  // (see `createJumpPin`).
  const [pin] = useState(createJumpPin)
  const settleTimersRef = useRef<ReturnType<typeof setTimeout>[]>([])
  const startJump = useCallback(
    (id: string) => {
      pin.jump(id)
      settleTimersRef.current.forEach(clearTimeout)
      const from = globalThis.scrollY
      settleTimersRef.current = [
        // A jump to a heading that is already in place moves nothing, so no
        // `scrollend` will ever settle it — and the reader's next scroll
        // would be mistaken for the jump's. If the page has not moved a
        // moment after the jump, there was no travel.
        setTimeout(() => {
          if (globalThis.scrollY === from) pin.settle()
        }, JUMP_START_GRACE_MS),
        setTimeout(pin.settle, JUMP_SETTLE_FALLBACK_MS),
      ]
    },
    [pin]
  )
  useEffect(
    () => () => {
      settleTimersRef.current.forEach(clearTimeout)
    },
    []
  )

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
      setActiveId(
        pin.current() ??
          pickActiveHeading(
            elements.map(el => ({
              id: el.id,
              top: el.getBoundingClientRect().top,
            })),
            READING_LINE_PX
          )
      )
    }
    // The end of a scroll: the jump's own (which settles the pin, so the next
    // scroll is the reader's) or any other (position decides again).
    const onScrollEnd = () => {
      pin.settle()
      update()
    }
    // Any scroll once the jump has settled is the reader's — a dragged
    // scrollbar and `scrollTo` included, which fire no input event. This runs
    // only while a pin is held and does no layout work: it clears a flag.
    const onScroll = () => {
      // The end-of-document exception: a late heading the page cannot scroll
      // up to the reading line stays the jump's target while the page is
      // still at its very end with that heading on screen. Late layout (a
      // diagram rendering, an image loading) fires scroll events there too,
      // and they are not the reader leaving.
      const pinned = pin.current()
      const target = pinned === null ? null : document.getElementById(pinned)
      if (target && atDocumentEnd() && isOnScreen(target)) return
      if (pin.scrolled()) update()
    }
    // Deliberate input releases the pin even mid-jump: the reader has taken
    // over the scroll.
    const onUserScroll = () => {
      if (pin.userScrolled()) update()
    }
    globalThis.addEventListener('scroll', onScroll, { passive: true })
    MANUAL_SCROLL_EVENTS.forEach(type => {
      globalThis.addEventListener(type, onUserScroll, { passive: true })
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
    globalThis.addEventListener('scrollend', onScrollEnd, { passive: true })
    elements.forEach(el => {
      observer.observe(el)
    })
    return () => {
      observer.disconnect()
      globalThis.removeEventListener('scrollend', onScrollEnd)
      globalThis.removeEventListener('scroll', onScroll)
      MANUAL_SCROLL_EVENTS.forEach(type => {
        globalThis.removeEventListener(type, onUserScroll)
      })
    }
  }, [articleRef, entries, pin])

  // A URL that arrives with a hash scrolls to its heading once the body has
  // rendered — the browser's own attempt ran before the heading existed. The
  // body is still settling at that point (a diagram renders, an image loads),
  // which moves the heading after we scrolled to it; so for a short while,
  // and only until the reader touches the scroll, a change in the article's
  // height scrolls to the heading again.
  // A ref, not state: marking it handled must not re-run this effect, whose
  // cleanup is what stops the settling observer.
  const hashHandledRef = useRef(false)
  useEffect(() => {
    const article = articleRef.current
    if (hashHandledRef.current || entries.length === 0 || !article) return
    hashHandledRef.current = true
    const id = decodeURIComponent(globalThis.location.hash.slice(1))
    if (!id || !entries.some(entry => entry.id === id)) return

    const scrollToTarget = () => {
      startJump(id)
      document.getElementById(id)?.scrollIntoView({ block: 'start' })
    }
    scrollToTarget()

    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(scrollToTarget)
    const stop = () => {
      observer.disconnect()
      clearTimeout(timer)
      MANUAL_SCROLL_EVENTS.forEach(type => {
        globalThis.removeEventListener(type, stop)
      })
    }
    const timer = setTimeout(stop, HASH_SETTLE_WINDOW_MS)
    MANUAL_SCROLL_EVENTS.forEach(type => {
      globalThis.addEventListener(type, stop, { passive: true })
    })
    observer.observe(article)
    return stop
  }, [articleRef, entries, startJump])

  const goTo = useCallback(
    (id: string) => {
      const target = document.getElementById(id)
      if (!target) return
      startJump(id)
      target.scrollIntoView({ block: 'start', behavior: 'smooth' })
      // replaceState, not a navigation: the section is linkable without adding
      // a history entry per click or re-running the router.
      globalThis.history.replaceState(
        globalThis.history.state,
        '',
        `#${encodeURIComponent(id)}`
      )
      setActiveId(id)
    },
    [startJump]
  )

  return {
    entries: enabled ? entries : [],
    activeId: entries.length > 0 ? activeId : null,
    goTo,
  }
}
