import { useAnimatedNumber } from './useAnimatedNumber'

/**
 * A number that counts toward its value instead of jumping. Renders as text,
 * so it works inside HTML and SVG `<text>` alike.
 */
export function AnimatedNumber({
  value,
  durationMs,
  disabled,
}: Readonly<{ value: number; durationMs: number; disabled?: boolean }>) {
  const shown = useAnimatedNumber(value, durationMs, disabled)
  return <>{shown.toLocaleString()}</>
}
