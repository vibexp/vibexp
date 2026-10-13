import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useLocalStorage } from '@/hooks/useLocalStorage'
import { BREAKPOINT_QUERIES, useMediaQuery } from '@/hooks/useMediaQuery'

/**
 * How the shell frames the current page (#886).
 *
 * - `contained` — the default: content is centered in the 1280px container
 *   with the standard page padding (list pages, forms, dashboards).
 * - `reading` — full-bleed: the page owns the whole content row and lays out
 *   its own article column + details column (every resource detail page).
 */
export type ShellContentMode = 'contained' | 'reading'

export interface ShellContextValue {
  /** Desktop (`lg+`) navigation expanded (264px) vs folded to the icon rail. */
  navExpanded: boolean
  toggleNav: () => void

  /** Viewport is at least `md` (768px). */
  isTablet: boolean
  /** Viewport is at least `lg` (1024px). */
  isDesktop: boolean
  /**
   * The details panel sits BESIDE the article, as a column. When false it is an
   * overlay drawer opened from the header toggle. A reading page asks for the
   * column only at `xl` (1280px) — below that 320px of rail costs the text too
   * much — while an edit page keeps it from `lg`, because its Save lives there.
   */
  detailsInline: boolean

  /** True while a page has registered a details panel via `useReadingShell`. */
  detailsRegistered: boolean
  /** Desktop details column open (320px) vs folded to the icon rail. Persisted. */
  detailsOpen: boolean
  setDetailsOpen: (open: boolean) => void
  toggleDetails: () => void
  /** Below `lg` the details live in a sheet; this is its (transient) open state. */
  detailsSheetOpen: boolean
  setDetailsSheetOpen: (open: boolean) => void

  contentMode: ShellContentMode

  /**
   * Focus mode: the navigation and the details panel are both hidden, leaving
   * the text. It is an overlay on the two stored preferences, never a change to
   * them, so leaving it restores each exactly as it was — and it is not stored,
   * so it never survives a page load.
   */
  focusMode: boolean
  setFocusMode: (on: boolean) => void

  /**
   * The name of the thing the current page is about (a resource's title), shown
   * as the breadcrumb's last crumb. `null` when the page did not name one.
   */
  breadcrumbLeaf: string | null
  /**
   * Every crumb the page named after its section, outermost first — usually
   * one (a resource), sometimes two (a team, then the tab of it on screen).
   * `breadcrumbLeaf` is its last entry.
   */
  breadcrumbTrail: readonly string[]
}

/** What a reading page tells the shell about itself. */
export interface ShellRegistration {
  /** Whether the page renders a details panel (drives the header toggle). */
  details: boolean
  /** The details column needs `xl`, not `lg`, to sit beside the article. */
  wideRail?: boolean
}

interface ShellInternalValue extends ShellContextValue {
  register: (registration: ShellRegistration) => () => void
  setBreadcrumbTrail: (trail: readonly string[]) => void
}

const noop = (): void => undefined

/**
 * Safe defaults used when a component renders outside `ShellProvider`
 * (unit tests, `BareLayout` routes): behave like an expanded desktop shell.
 */
const DEFAULT_SHELL: ShellInternalValue = {
  navExpanded: true,
  toggleNav: noop,
  isTablet: true,
  isDesktop: true,
  detailsInline: true,
  detailsRegistered: false,
  detailsOpen: true,
  setDetailsOpen: noop,
  toggleDetails: noop,
  detailsSheetOpen: false,
  setDetailsSheetOpen: noop,
  contentMode: 'contained',
  focusMode: false,
  setFocusMode: noop,
  breadcrumbLeaf: null,
  breadcrumbTrail: [],
  register: () => noop,
  setBreadcrumbTrail: noop,
}

const ShellContext = createContext<ShellInternalValue>(DEFAULT_SHELL)

export function ShellProvider({ children }: Readonly<{ children: ReactNode }>) {
  const isTablet = useMediaQuery(BREAKPOINT_QUERIES.md)
  const isDesktop = useMediaQuery(BREAKPOINT_QUERIES.lg)
  const isWide = useMediaQuery(BREAKPOINT_QUERIES.xl)

  // Both collapse states are stored as "collapsed" booleans so the absence of
  // a key (first visit) yields the design default: everything open.
  const [navCollapsed, setNavCollapsed] = useLocalStorage<boolean>(
    STORAGE_KEYS.NAV_COLLAPSED,
    false
  )
  const [detailsCollapsed, setDetailsCollapsed] = useLocalStorage<boolean>(
    STORAGE_KEYS.DETAILS_COLLAPSED,
    false
  )
  const [detailsSheetOpen, setDetailsSheetOpen] = useState(false)
  const [breadcrumbTrail, setBreadcrumbTrail] = useState<readonly string[]>([])
  const breadcrumbLeaf = breadcrumbTrail.at(-1) ?? null

  // A page registers once on mount and unregisters on unmount, so counters
  // (not booleans) survive the brief overlap when one reading page replaces
  // another during navigation.
  const [readingPages, setReadingPages] = useState(0)
  const [detailsPanels, setDetailsPanels] = useState(0)
  const [wideRails, setWideRails] = useState(0)
  const [focusMode, setFocusMode] = useState(false)

  const register = useCallback((registration: ShellRegistration) => {
    const { details, wideRail = false } = registration
    setReadingPages(n => n + 1)
    if (details) setDetailsPanels(n => n + 1)
    if (wideRail) setWideRails(n => n + 1)
    return () => {
      setReadingPages(n => n - 1)
      if (details) setDetailsPanels(n => n - 1)
      if (wideRail) setWideRails(n => n - 1)
    }
  }, [])

  const detailsRegistered = detailsPanels > 0
  const contentMode: ShellContentMode =
    readingPages > 0 ? 'reading' : 'contained'

  const detailsInline = wideRails > 0 ? isWide : isDesktop

  // The sheet only exists while the details are not a column; drop it when a
  // page without details takes over or the viewport grows into the column
  // layout.
  useEffect(() => {
    if (!detailsRegistered || detailsInline) setDetailsSheetOpen(false)
  }, [detailsRegistered, detailsInline])

  // Focus mode belongs to the reading page that turned it on.
  useEffect(() => {
    if (readingPages === 0) setFocusMode(false)
  }, [readingPages])

  const value = useMemo<ShellInternalValue>(
    () => ({
      navExpanded: !navCollapsed,
      toggleNav: () => {
        setNavCollapsed(c => !c)
      },
      isTablet,
      isDesktop,
      detailsInline,
      detailsRegistered,
      detailsOpen: !detailsCollapsed,
      setDetailsOpen: open => {
        setDetailsCollapsed(!open)
      },
      toggleDetails: () => {
        setDetailsCollapsed(c => !c)
      },
      detailsSheetOpen,
      setDetailsSheetOpen,
      contentMode,
      focusMode,
      setFocusMode,
      breadcrumbLeaf,
      breadcrumbTrail,
      register,
      setBreadcrumbTrail,
    }),
    [
      navCollapsed,
      setNavCollapsed,
      detailsCollapsed,
      setDetailsCollapsed,
      isTablet,
      isDesktop,
      detailsInline,
      detailsRegistered,
      detailsSheetOpen,
      contentMode,
      focusMode,
      breadcrumbLeaf,
      breadcrumbTrail,
      register,
    ]
  )

  return <ShellContext.Provider value={value}>{children}</ShellContext.Provider>
}

/** Read the shell state. Safe outside a provider (returns desktop defaults). */
export function useShell(): ShellContextValue {
  return useContext(ShellContext)
}

/**
 * Called by the reading-page layout: switches the shell into `reading` mode
 * and (when `details` is true) surfaces the details toggle in the header for
 * as long as the page is mounted.
 */
export function useReadingShell(registration: ShellRegistration): void {
  const { register } = useContext(ShellContext)
  const { details, wideRail = false } = registration
  useEffect(
    () => register({ details, wideRail }),
    [register, details, wideRail]
  )
}

/** Separates the crumbs of a trail in an effect key; never in a label. */
const TRAIL_KEY_SEPARATOR = '\u0000'

/**
 * Names the thing the current page is about, for the breadcrumb's last crumb
 * ("VibeXP / Artifacts / <title>"), and for the tab title. Pass several labels
 * for a page inside a named thing ("VibeXP / Teams / <team> / Projects").
 * Cleared when the page unmounts or stops naming one; blank labels are
 * dropped, and a page that names nothing leaves the breadcrumb at its section.
 */
export function useBreadcrumbLeaf(
  label: string | readonly (string | null | undefined)[] | null | undefined
): void {
  const { setBreadcrumbTrail } = useContext(ShellContext)
  const labels = typeof label === 'string' ? [label] : (label ?? [])
  // A string key, so a fresh array of the same labels is not a new trail.
  const key = labels
    .map(part => part?.trim() ?? '')
    .filter(part => part !== '')
    .join(TRAIL_KEY_SEPARATOR)
  useEffect(() => {
    setBreadcrumbTrail(key === '' ? [] : key.split(TRAIL_KEY_SEPARATOR))
    return () => {
      setBreadcrumbTrail([])
    }
  }, [setBreadcrumbTrail, key])
}
