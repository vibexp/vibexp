import { useEffect } from 'react'
import { useLocation } from 'react-router'

import { resolveBreadcrumb } from '@/components/layout/HeaderBreadcrumb'
import { useShell } from '@/components/layout/ShellContext'
import { siteTitleSuffix } from '@/features/site-info/siteInfo'
import { useSiteInfo } from '@/features/site-info/useSiteInfo'

const SEPARATOR = ' · '

/**
 * The browser tab's title for a page: most specific first, so a row of tabs
 * and a history list can be told apart by their first words.
 *
 * - a page about one thing: `<resource> · <section> · <site>`
 * - a page inside a named thing: `<tab> · <team> · <section> · <site>`
 * - any other page: `<section> · <site>`
 * - a page the shell cannot name: `<site>`
 */
export function buildDocumentTitle(parts: {
  /** The crumbs after the section, outermost first, as the breadcrumb shows them. */
  trail: readonly string[]
  section: string | null
  site: string
}): string {
  return [...[...parts.trail].reverse(), parts.section, parts.site]
    .filter((part): part is string => Boolean(part?.trim()))
    .join(SEPARATOR)
}

/**
 * Sets `document.title` for every page inside the app shell, once, from what
 * the breadcrumb already resolves: the section the route belongs to and the
 * crumbs after it. No page sets a title by hand, and a new resource page gets
 * one for free.
 *
 * The title ends the way every tab title does (`siteTitleSuffix`): the
 * instance's company name where one is set, and always VibeXP.
 */
export function useDocumentTitle(): void {
  const { pathname } = useLocation()
  const { breadcrumbTrail } = useShell()
  const { company_name } = useSiteInfo()

  const { section, trail } = resolveBreadcrumb(pathname, breadcrumbTrail)
  const title = buildDocumentTitle({
    trail,
    section: section?.label ?? null,
    site: siteTitleSuffix(company_name),
  })

  useEffect(() => {
    document.title = title
  }, [title])
}
