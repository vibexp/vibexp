/**
 * Avatar palette decision (issue #1647, design-system readiness):
 *
 * The previous 10-colour Tailwind palette (rose/amber/emerald/sky/violet/…) has
 * no equivalent in the monochrome `@vibexp/design-system`. Of the three
 * options considered, we adopted **Option B**: map avatars onto the design
 * system's sanctioned categorical palette — the `--chart-1..5` tokens. This
 * keeps avatar colours on-token (so they survive the eventual token-source
 * swap with no further change), removes the last hardcoded palette from the
 * avatar path, and stays within the design language. Five distinct hues are
 * ample for decorative identity avatars — the initial disambiguates the
 * occasional colour collision.
 */
const PALETTE = [
  'bg-chart-1',
  'bg-chart-2',
  'bg-chart-3',
  'bg-chart-4',
  'bg-chart-5',
]

/**
 * Returns a deterministic Tailwind background-colour class for the given
 * assistant name. The same name always maps to the same colour.
 */
export function assistantColor(name: string): string {
  const index =
    Array.from(name).reduce(
      (acc, char) => acc + (char.codePointAt(0) ?? 0),
      0
    ) % PALETTE.length
  return PALETTE[index]
}

/**
 * Returns the uppercased first character of the assistant name, used as the
 * avatar initial. Falls back to "?" when the name is empty.
 */
export function assistantInitial(name: string): string {
  return name.length > 0 ? name[0].toUpperCase() : '?'
}
