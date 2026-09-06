import type { ReactNode } from 'react'

import { PanelPresentationProvider } from '@/components/ui/panel'

/**
 * Test helper for the reading page's flat details column (#890): wraps `ui` in
 * the surface `DetailsColumn` provides, so a widget renders exactly as it does
 * inside the column.
 */
export function FlatSurface({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <PanelPresentationProvider value="flat">
      {children}
    </PanelPresentationProvider>
  )
}

/**
 * The card chrome a widget must NOT paint inside the details column — the
 * column is already a bordered, padded surface, so a card in it is a second
 * border around the same content.
 */
const CARD_CHROME_CLASSES = ['bg-card', 'rounded-lg', 'shadow-sm'] as const

/**
 * Asserts no element in `root` carries card chrome. Deliberately not "no
 * border/radius anywhere": the design keeps hairline row dividers and the
 * compact chip actions, badges and code chips, which are borders and radii of
 * their own. What must go is the *card box*.
 */
export function expectNoCardChrome(root: HTMLElement): void {
  const offenders: string[] = []
  for (const el of [root, ...Array.from(root.querySelectorAll('*'))]) {
    for (const cls of CARD_CHROME_CLASSES) {
      if (el.classList.contains(cls)) {
        offenders.push(`${el.tagName.toLowerCase()}.${cls}`)
      }
    }
  }
  expect(offenders).toEqual([])
}
