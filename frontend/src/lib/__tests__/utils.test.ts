import { cn } from '@/lib/utils'

describe('cn', () => {
  it('lets a later design-system width token replace a base width', () => {
    // SelectTrigger ships `w-full`; a filter control passes `w-control-sm`.
    expect(cn('flex w-full', 'w-control-sm')).toBe('flex w-control-sm')
    // The tablet details Sheet ships `w-3/4`; ReadingPage passes the token.
    expect(cn('w-3/4 sm:max-w-sm', 'w-details-column')).toBe(
      'sm:max-w-sm w-details-column'
    )
    expect(cn('w-12', 'w-rail-expanded')).toBe('w-rail-expanded')
  })

  it('merges min/max width tokens against the default scale', () => {
    expect(cn('min-w-0', 'min-w-control-search-min')).toBe(
      'min-w-control-search-min'
    )
    expect(cn('max-w-sm', 'max-w-control-search-max')).toBe(
      'max-w-control-search-max'
    )
  })

  it('still merges the default scale as before', () => {
    expect(cn('w-80', 'w-3/4')).toBe('w-3/4')
    expect(cn('px-2', 'px-4')).toBe('px-4')
  })
})
