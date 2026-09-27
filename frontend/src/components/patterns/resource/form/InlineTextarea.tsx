import type { TextareaHTMLAttributes } from 'react'
import { useEffect, useLayoutEffect, useRef } from 'react'

/** Whether the browser grows a textarea to its content by CSS alone. */
function supportsFieldSizing(): boolean {
  return (
    typeof CSS !== 'undefined' &&
    typeof CSS.supports === 'function' &&
    CSS.supports('field-sizing', 'content')
  )
}

function fitToContent(el: HTMLTextAreaElement) {
  el.style.height = 'auto'
  el.style.height = `${String(el.scrollHeight)}px`
}

/**
 * A header textarea that is always exactly as tall as its text.
 *
 * `field-sizing: content` does that in CSS; a browser without it would keep
 * the one-row box and hide every wrapped line under `overflow-hidden`, so there
 * the height is set from `scrollHeight` whenever the value changes. `FormControl`
 * clones its id / `aria-*` onto this component, which passes them to the leaf.
 */
export function InlineTextarea({
  value,
  ...props
}: Readonly<TextareaHTMLAttributes<HTMLTextAreaElement> & { value: string }>) {
  const ref = useRef<HTMLTextAreaElement>(null)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el || supportsFieldSizing()) return
    fitToContent(el)
  }, [value])
  // A narrower column wraps the same text onto more lines, so the fallback
  // re-fits on a WIDTH change too. Its own height writes resize the box as
  // well; comparing widths keeps those from looping.
  useEffect(() => {
    const el = ref.current
    if (!el || supportsFieldSizing() || typeof ResizeObserver === 'undefined') {
      return
    }
    let width = el.clientWidth
    const observer = new ResizeObserver(() => {
      if (el.clientWidth === width) return
      width = el.clientWidth
      fitToContent(el)
    })
    observer.observe(el)
    return () => {
      observer.disconnect()
    }
  }, [])
  // `ref` last: `FormControl`'s Slot hands down a ref prop of its own (React
  // 19 passes refs as props), which must not replace the one measured here.
  return <textarea rows={1} {...props} value={value} ref={ref} />
}
