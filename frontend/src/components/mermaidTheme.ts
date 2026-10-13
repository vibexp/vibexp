/**
 * Mermaid theming from the app's own colour tokens.
 *
 * Mermaid's `base` theme is the only one that honours `themeVariables`, and it
 * derives every colour it is not given from `primaryColor` by shifting hue —
 * which, starting from a neutral, stays neutral. So a handful of tokens is
 * enough to draw every diagram type in the product's monochrome palette, in
 * whichever theme is active.
 *
 * The tokens are OKLCH (and `--border` is translucent in dark mode), neither of
 * which mermaid's colour parser understands, so each one is resolved to an
 * opaque `rgb()` by painting it over the page background on a canvas.
 */

/** Converts any CSS colour, painted over `backdrop`, to an opaque `rgb()`. */
export type ColorResolver = (color: string, backdrop?: string) => string | null

export interface MermaidTheme {
  darkMode: boolean
  themeVariables: Record<string, string | boolean>
}

/** Mermaid `themeVariables` key -> the design token it is drawn from. */
const TOKEN_BY_VARIABLE = {
  background: '--background',
  // Nodes, actors, classes, states.
  primaryColor: '--muted',
  primaryTextColor: '--foreground',
  primaryBorderColor: '--muted-foreground',
  // Alternate fills (activations, alternating rows, composite states).
  secondaryColor: '--accent',
  secondaryTextColor: '--foreground',
  secondaryBorderColor: '--border',
  // Group backgrounds (subgraphs, namespaces).
  tertiaryColor: '--card',
  tertiaryTextColor: '--foreground',
  tertiaryBorderColor: '--border',
  // Arrows, lifelines, and every label that sits on the page itself.
  lineColor: '--foreground',
  textColor: '--foreground',
  titleColor: '--foreground',
  edgeLabelBackground: '--background',
  // Notes default to a fixed yellow in `base`; keep them in the palette.
  noteBkgColor: '--accent',
  noteTextColor: '--foreground',
  noteBorderColor: '--border',
  // Pie slices must differ from each other, which shades derived from one
  // neutral do not: they take the app's chart palette instead.
  pie1: '--chart-1',
  pie2: '--chart-2',
  pie3: '--chart-3',
  pie4: '--chart-4',
  pie5: '--chart-5',
  pieStrokeColor: '--background',
  pieOuterStrokeColor: '--border',
} as const

let canvasContext: CanvasRenderingContext2D | null | undefined

const canvasResolver: ColorResolver = (color, backdrop) => {
  canvasContext ??= document
    .createElement('canvas')
    .getContext('2d', { willReadFrequently: true })
  const ctx = canvasContext
  if (!ctx) return null

  ctx.clearRect(0, 0, 1, 1)
  for (const layer of [backdrop, color]) {
    if (!layer) continue
    // An unparseable colour leaves `fillStyle` untouched; start from a known
    // value so that case is detectable rather than silently painting black.
    ctx.fillStyle = '#000000'
    ctx.fillStyle = layer
    ctx.fillRect(0, 0, 1, 1)
  }
  const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data
  if (a === 0) return null
  return `rgb(${String(r)}, ${String(g)}, ${String(b)})`
}

const isDark = (rgb: string): boolean => {
  const channels = rgb.match(/\d+/g)?.map(Number) ?? []
  if (channels.length < 3) return false
  const [r, g, b] = channels
  // Perceived brightness (ITU-R BT.601), 0–255.
  return 0.299 * r + 0.587 * g + 0.114 * b < 128
}

/**
 * Reads the mermaid theme for the colour tokens in effect at `scope`.
 *
 * A token that is not defined there (or cannot be resolved) is left out, so
 * mermaid falls back to its own value for it — which is also what happens
 * wherever there is no stylesheet at all, as under jsdom.
 */
export function readMermaidTheme(
  scope: Element,
  resolve: ColorResolver = canvasResolver
): MermaidTheme {
  const style = getComputedStyle(scope)
  const token = (name: string) => style.getPropertyValue(name).trim()

  const backgroundToken = token(TOKEN_BY_VARIABLE.background)
  const background = backgroundToken ? resolve(backgroundToken) : null

  const themeVariables: MermaidTheme['themeVariables'] = {}
  for (const [variable, name] of Object.entries(TOKEN_BY_VARIABLE)) {
    const value = token(name)
    if (!value) continue
    const rgb = resolve(value, background ?? undefined)
    if (rgb) themeVariables[variable] = rgb
  }

  const darkMode = background ? isDark(background) : false
  // `base` reads the flag from the variables as well as from the config.
  if (background) themeVariables.darkMode = darkMode

  return { darkMode, themeVariables }
}
