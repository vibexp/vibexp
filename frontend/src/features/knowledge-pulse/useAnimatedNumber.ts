import { useEffect, useRef, useState } from 'react'

/**
 * Mostly linear so the count keeps visibly climbing across the whole
 * duration, with a little ease-out so it settles rather than stops dead.
 */
function ease(t: number): number {
  return 0.7 * t + 0.3 * (1 - Math.pow(1 - t, 3))
}

/**
 * The value to render for a number that should count toward `target`
 * instead of jumping to it. Each new target starts a tween from whatever is
 * on screen right now, so a poll that lands mid-count just re-aims it and
 * the counter never stalls or jumps back.
 *
 * Pass the poll interval as `durationMs` and the counter keeps ticking for
 * the whole gap between polls, arriving as the next one does.
 */
export function useAnimatedNumber(
  target: number,
  durationMs: number,
  disabled = false
): number {
  const [display, setDisplay] = useState(target)
  const displayRef = useRef(target)
  const frameRef = useRef<number | undefined>(undefined)

  useEffect(() => {
    const from = displayRef.current
    if (disabled || from === target) {
      displayRef.current = target
      setDisplay(target)
      return
    }
    const start = performance.now()
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / durationMs)
      const value = from + (target - from) * ease(t)
      displayRef.current = value
      setDisplay(value)
      if (t < 1) frameRef.current = requestAnimationFrame(tick)
    }
    frameRef.current = requestAnimationFrame(tick)
    return () => {
      if (frameRef.current !== undefined) cancelAnimationFrame(frameRef.current)
    }
  }, [target, durationMs, disabled])

  return Math.round(display)
}
