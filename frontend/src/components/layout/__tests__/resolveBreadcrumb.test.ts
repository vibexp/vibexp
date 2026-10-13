import { resolveBreadcrumb } from '../HeaderBreadcrumb'

describe('resolveBreadcrumb — static sub-pages', () => {
  it('names a static sub-page, with or without trailing slashes', () => {
    expect(resolveBreadcrumb('/settings/api-keys', []).trail).toEqual([
      'API keys',
    ])
    expect(resolveBreadcrumb('/settings/api-keys///', []).trail).toEqual([
      'API keys',
    ])
  })

  it('prefers the crumbs the page named itself', () => {
    expect(resolveBreadcrumb('/settings/api-keys', ['Mine']).trail).toEqual([
      'Mine',
    ])
  })

  it('stays fast on a path that is mostly slashes', () => {
    const path = `${'/'.repeat(50_000)}a`
    const started = performance.now()
    expect(resolveBreadcrumb(path, []).trail).toEqual([])
    expect(performance.now() - started).toBeLessThan(1000)
  })
})
