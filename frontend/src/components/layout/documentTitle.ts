import { useEffect } from 'react'
import { useLocation } from 'react-router'

import { currentPageCrumb } from '@/components/layout/HeaderBreadcrumb'
import { useShell } from '@/components/layout/ShellContext'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'

const SEPARATOR = ' · '

/**
 * Sections that are pages in their own right but have no navigation entry for
 * the breadcrumb to resolve them from.
 */
const UNLISTED_SECTIONS = new Map<string, string>([
  ['/notifications', 'Notifications'],
])

/**
 * The browser tab's title for a page: most specific first, so a row of tabs
 * and a history list can be told apart by their first words.
 *
 * - a page about one thing: `<resource> · <section> · <site>`
 * - any other page: `<section> · <site>`
 * - a page the shell cannot name: `<site>`
 */
export function buildDocumentTitle(parts: {
  leaf: string | null
  section: string | null
  site: string
}): string {
  return [parts.leaf, parts.section, parts.site]
    .filter((part): part is string => Boolean(part?.trim()))
    .join(SEPARATOR)
}

/**
 * Sets `document.title` for every page inside the app shell, once, from what
 * the shell already knows: the section the route belongs to (the breadcrumb's
 * own resolution) and the resource a page named with `useBreadcrumbLeaf`. No
 * page sets a title by hand, and a new resource page gets one for free.
 *
 * The site name is the instance's own (site info) where it is set, VibeXP
 * otherwise.
 */
export function useDocumentTitle(): void {
  const { pathname } = useLocation()
  const { breadcrumbLeaf } = useShell()
  const { displayName } = useSiteInfo()

  const section =
    currentPageCrumb(pathname)?.label ?? UNLISTED_SECTIONS.get(pathname) ?? null
  const title = buildDocumentTitle({
    leaf: breadcrumbLeaf,
    section,
    site: displayName,
  })

  useEffect(() => {
    document.title = title
  }, [title])
}
