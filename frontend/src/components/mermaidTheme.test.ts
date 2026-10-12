import type { ColorResolver } from './mermaidTheme'
import { readMermaidTheme } from './mermaidTheme'

const LIGHT = {
  '--background': 'oklch(1 0 0)',
  '--foreground': 'oklch(0.145 0 0)',
  '--muted': 'oklch(0.97 0 0)',
  '--muted-foreground': 'oklch(0.556 0 0)',
  '--border': 'oklch(0.922 0 0)',
  '--chart-1': 'oklch(0.646 0.222 41.116)',
}

/** Stands in for the canvas: a lookup from token value to an rgb() string. */
const RGB: Record<string, string> = {
  'oklch(1 0 0)': 'rgb(255, 255, 255)',
  'oklch(0.145 0 0)': 'rgb(10, 10, 10)',
  'oklch(0.97 0 0)': 'rgb(245, 245, 245)',
  'oklch(0.556 0 0)': 'rgb(115, 115, 115)',
  'oklch(0.922 0 0)': 'rgb(229, 229, 229)',
  'oklch(0.646 0.222 41.116)': 'rgb(245, 74, 0)',
  'oklch(1 0 0 / 10%)': 'rgb(38, 38, 38)',
}

function stubTokens(tokens: Record<string, string>) {
  vi.spyOn(window, 'getComputedStyle').mockReturnValue({
    getPropertyValue: (name: string) =>
      Object.hasOwn(tokens, name) ? ` ${tokens[name]} ` : '',
  } as CSSStyleDeclaration)
}

const scope = document.createElement('div')

describe('readMermaidTheme', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('maps the colour tokens onto mermaid theme variables', () => {
    stubTokens(LIGHT)
    const resolve: ColorResolver = color => RGB[color] ?? null

    const theme = readMermaidTheme(scope, resolve)

    expect(theme.darkMode).toBe(false)
    expect(theme.themeVariables).toMatchObject({
      darkMode: false,
      background: 'rgb(255, 255, 255)',
      primaryColor: 'rgb(245, 245, 245)',
      primaryTextColor: 'rgb(10, 10, 10)',
      primaryBorderColor: 'rgb(115, 115, 115)',
      lineColor: 'rgb(10, 10, 10)',
      noteBorderColor: 'rgb(229, 229, 229)',
      pie1: 'rgb(245, 74, 0)',
    })
  })

  it('reports dark mode from the resolved background, not from a class name', () => {
    stubTokens({ ...LIGHT, '--background': 'oklch(0.145 0 0)' })

    const theme = readMermaidTheme(scope, color => RGB[color] ?? null)

    expect(theme.darkMode).toBe(true)
    expect(theme.themeVariables.darkMode).toBe(true)
  })

  it('paints every token over the page background, so a translucent one comes out opaque', () => {
    stubTokens({
      '--background': 'oklch(0.145 0 0)',
      '--border': 'oklch(1 0 0 / 10%)',
    })
    const resolve = vi.fn<ColorResolver>(color => RGB[color] ?? null)

    const theme = readMermaidTheme(scope, resolve)

    expect(resolve).toHaveBeenCalledWith(
      'oklch(1 0 0 / 10%)',
      'rgb(10, 10, 10)'
    )
    expect(theme.themeVariables.noteBorderColor).toBe('rgb(38, 38, 38)')
  })

  it('leaves out a token that is undefined or cannot be resolved', () => {
    stubTokens({ '--background': 'oklch(1 0 0)', '--muted': 'not-a-colour' })

    const theme = readMermaidTheme(scope, color => RGB[color] ?? null)

    expect(theme.themeVariables).not.toHaveProperty('primaryColor')
    expect(theme.themeVariables).not.toHaveProperty('lineColor')
  })

  it('falls back to mermaid defaults where there is no stylesheet at all', () => {
    // No stub: jsdom defines none of the tokens, and the canvas is never
    // touched because there is nothing to resolve.
    expect(readMermaidTheme(scope)).toEqual({
      darkMode: false,
      themeVariables: {},
    })
  })
})
