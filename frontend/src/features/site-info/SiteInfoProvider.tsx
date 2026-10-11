import { useEffect, useMemo, useRef, useState } from 'react'

import { siteInfoService } from '@/services/siteInfoService'

import {
  deriveSiteInfo,
  readInlinedSiteInfo,
  SITE_INFO_SAVED_EVENT,
  type SiteInfo,
  toSiteInfo,
} from './siteInfo'
import { SiteInfoContext } from './SiteInfoContext'

/**
 * Makes the instance's site info available to the whole app.
 *
 * The initial value is read synchronously from the global `/config.js` set, so
 * the very first render is already branded and there is no loading state. It
 * does not fetch on mount: the inlined value is as fresh as the page load.
 * After an instance admin saves (`notifySiteInfoSaved`), it refetches the
 * public endpoint. A failed refetch keeps the current values: branding is
 * never worth an error on screen.
 */
export function SiteInfoProvider({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const [info, setInfo] = useState<SiteInfo>(readInlinedSiteInfo)
  // Newest-request-wins: a slow response to an earlier save must not
  // overwrite the result of a later one, nor land after unmount.
  const latestRequest = useRef(0)

  useEffect(() => {
    const refetch = () => {
      const request = ++latestRequest.current
      siteInfoService
        .getSiteInfo()
        .then(fresh => {
          if (latestRequest.current === request) setInfo(toSiteInfo(fresh))
        })
        .catch((error: unknown) => {
          console.error('Failed to refresh the site info', error)
        })
    }
    window.addEventListener(SITE_INFO_SAVED_EVENT, refetch)
    return () => {
      window.removeEventListener(SITE_INFO_SAVED_EVENT, refetch)
      latestRequest.current += 1
    }
  }, [])

  const value = useMemo(() => deriveSiteInfo(info), [info])

  return (
    <SiteInfoContext.Provider value={value}>
      {children}
    </SiteInfoContext.Provider>
  )
}
