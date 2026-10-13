import { afterEach, describe, expect, it } from 'vitest'

import { headerHeightPx } from '../headerHeight'

describe('headerHeightPx', () => {
  afterEach(() => {
    document.documentElement.style.removeProperty('--header-height')
    document.documentElement.style.removeProperty('font-size')
  })

  it('falls back to the token’s documented 56px when no stylesheet defines it', () => {
    expect(headerHeightPx()).toBe(56)
  })

  it('follows the token and the root font size', () => {
    document.documentElement.style.setProperty('--header-height', '4rem')
    document.documentElement.style.fontSize = '20px'

    expect(headerHeightPx()).toBe(80)
  })
})
