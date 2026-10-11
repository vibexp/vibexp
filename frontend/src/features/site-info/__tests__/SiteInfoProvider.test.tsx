import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { notifySiteInfoSaved, type SiteInfo } from '../siteInfo'
import { SiteInfoProvider } from '../SiteInfoProvider'
import { useSiteInfo } from '../useSiteInfo'

const { mockGetSiteInfo } = vi.hoisted(() => ({ mockGetSiteInfo: vi.fn() }))

vi.mock('@/services/siteInfoService', () => ({
  siteInfoService: { getSiteInfo: mockGetSiteInfo },
}))

const NOTHING: SiteInfo = {
  company_name: null,
  company_email: null,
  website: null,
  support_url: null,
  terms_url: null,
  privacy_url: null,
  logo_url: null,
  logo_dark_url: null,
}

/** Prints what a consumer sees, so the tests read it off the DOM. */
function Probe() {
  const { displayName, supportHref, hasCompanyBranding } = useSiteInfo()
  return (
    <div>
      <span data-testid="name">{displayName}</span>
      <span data-testid="support">{supportHref ?? 'none'}</span>
      <span data-testid="branded">{String(hasCompanyBranding)}</span>
    </div>
  )
}

function renderProvider() {
  return render(
    <SiteInfoProvider>
      <Probe />
    </SiteInfoProvider>
  )
}

/** A promise the test settles by hand, to order responses deliberately. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('SiteInfoProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    delete window.__VIBEXP_SITE_INFO__
    vi.restoreAllMocks()
  })

  it('is seeded from the /config.js global on the first render, with no fetch', () => {
    window.__VIBEXP_SITE_INFO__ = {
      ...NOTHING,
      company_name: 'Acme Inc.',
      company_email: 'hello@acme.test',
    }

    renderProvider()

    expect(screen.getByTestId('name')).toHaveTextContent('Acme Inc.')
    expect(screen.getByTestId('support')).toHaveTextContent(
      'mailto:hello@acme.test'
    )
    expect(screen.getByTestId('branded')).toHaveTextContent('true')
    expect(mockGetSiteInfo).not.toHaveBeenCalled()
  })

  it('starts as nothing configured when the global is absent (local dev)', () => {
    renderProvider()

    expect(screen.getByTestId('name')).toHaveTextContent('VibeXP')
    expect(screen.getByTestId('support')).toHaveTextContent('none')
    expect(screen.getByTestId('branded')).toHaveTextContent('false')
  })

  it('refetches when the site info is saved and updates consumers', async () => {
    mockGetSiteInfo.mockResolvedValue({ ...NOTHING, company_name: 'Acme Inc.' })
    renderProvider()
    expect(screen.getByTestId('name')).toHaveTextContent('VibeXP')

    act(() => {
      notifySiteInfoSaved()
    })

    await waitFor(() => {
      expect(screen.getByTestId('name')).toHaveTextContent('Acme Inc.')
    })
    expect(mockGetSiteInfo).toHaveBeenCalledTimes(1)
  })

  it('keeps the current values when the refetch fails', async () => {
    window.__VIBEXP_SITE_INFO__ = { ...NOTHING, company_name: 'Acme Inc.' }
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    mockGetSiteInfo.mockRejectedValue(new Error('network down'))
    renderProvider()

    act(() => {
      notifySiteInfoSaved()
    })

    await waitFor(() => {
      expect(consoleError).toHaveBeenCalledTimes(1)
    })
    expect(screen.getByTestId('name')).toHaveTextContent('Acme Inc.')
  })

  it('sanitises what the endpoint returns', async () => {
    mockGetSiteInfo.mockResolvedValue({
      ...NOTHING,
      company_name: 'Acme Inc.',
      support_url: 'javascript:alert(1)',
    })
    renderProvider()

    act(() => {
      notifySiteInfoSaved()
    })

    await waitFor(() => {
      expect(screen.getByTestId('name')).toHaveTextContent('Acme Inc.')
    })
    expect(screen.getByTestId('support')).toHaveTextContent('none')
  })

  it('lets the newest save win when responses arrive out of order', async () => {
    const first = deferred<SiteInfo>()
    const second = deferred<SiteInfo>()
    mockGetSiteInfo
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    renderProvider()

    act(() => {
      notifySiteInfoSaved()
      notifySiteInfoSaved()
    })
    await act(async () => {
      second.resolve({ ...NOTHING, company_name: 'Second' })
      await second.promise
    })
    await act(async () => {
      first.resolve({ ...NOTHING, company_name: 'First' })
      await first.promise
    })

    expect(screen.getByTestId('name')).toHaveTextContent('Second')
  })

  it('stops listening once unmounted', () => {
    const { unmount } = renderProvider()
    unmount()

    notifySiteInfoSaved()

    expect(mockGetSiteInfo).not.toHaveBeenCalled()
  })
})

describe('useSiteInfo outside a provider', () => {
  it('answers nothing configured instead of throwing', () => {
    render(<Probe />)

    expect(screen.getByTestId('name')).toHaveTextContent('VibeXP')
    expect(screen.getByTestId('support')).toHaveTextContent('none')
  })
})
