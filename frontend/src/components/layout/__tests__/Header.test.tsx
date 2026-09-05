import { render, screen } from '@testing-library/react'
import { userEvent } from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

import { Header } from '../Header'
import { ShellProvider, useReadingShell, useShell } from '../ShellContext'

vi.mock('@/components/layout/TeamSwitcher', () => ({
  TeamSwitcher: () => <div data-testid="team-switcher" />,
}))
vi.mock('@/components/layout/ProjectSwitcher', () => ({
  ProjectSwitcher: () => <div data-testid="project-switcher" />,
}))
vi.mock('@/components/layout/SearchModal', () => ({
  SearchModal: () => <button type="button">Search</button>,
}))
vi.mock('@/components/layout/ThemeToggle', () => ({
  ThemeToggle: () => <button type="button">Toggle theme</button>,
}))
vi.mock('@/components/layout/NotificationBell', () => ({
  NotificationBell: () => <button type="button">Notifications</button>,
}))
vi.mock('@/components/layout/UserMenu', () => ({
  UserMenu: () => <button type="button">User menu</button>,
}))
vi.mock('@/components/layout/MobileSidebar', () => ({
  MobileSidebar: () => <div data-testid="mobile-sidebar" />,
}))

function ReadingPageStub() {
  useReadingShell({ details: true })
  return null
}

function Probe() {
  const { navExpanded, detailsOpen, detailsSheetOpen } = useShell()
  return (
    <div
      data-testid="probe"
      data-nav={navExpanded ? 'expanded' : 'collapsed'}
      data-details={detailsOpen ? 'open' : 'collapsed'}
      data-sheet={detailsSheetOpen ? 'open' : 'closed'}
    />
  )
}

function renderHeader({ reading = false }: { reading?: boolean } = {}) {
  return render(
    <MemoryRouter initialEntries={['/artifacts']}>
      <ShellProvider>
        <Header />
        <Probe />
        {reading && <ReadingPageStub />}
      </ShellProvider>
    </MemoryRouter>
  )
}

describe('Header', () => {
  let viewport: ReturnType<typeof mockViewportWidth>

  beforeEach(() => {
    storage.clear()
    viewport = mockViewportWidth(1280)
  })

  afterEach(() => {
    viewport.restore()
  })

  describe('desktop (≥ 1024px)', () => {
    it('shows the nav toggle, the switchers, and no hamburger', () => {
      renderHeader()
      expect(screen.getByTestId('nav-toggle')).toBeInTheDocument()
      expect(screen.getByTestId('team-switcher')).toBeInTheDocument()
      expect(screen.getByTestId('project-switcher')).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Open navigation' })
      ).not.toBeInTheDocument()
    })

    it('folds and expands the navigation', async () => {
      const user = userEvent.setup()
      renderHeader()
      const toggle = screen.getByTestId('nav-toggle')
      expect(toggle).toHaveAccessibleName('Collapse navigation')
      await user.click(toggle)
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-nav',
        'collapsed'
      )
      expect(screen.getByTestId('nav-toggle')).toHaveAccessibleName(
        'Expand navigation'
      )
    })

    it('shows the details toggle only while a reading page registered one', () => {
      const { unmount } = renderHeader()
      expect(screen.queryByTestId('details-toggle')).not.toBeInTheDocument()
      unmount()
      renderHeader({ reading: true })
      expect(screen.getByTestId('details-toggle')).toBeInTheDocument()
    })

    it('folds the details column from the header', async () => {
      const user = userEvent.setup()
      renderHeader({ reading: true })
      const toggle = screen.getByTestId('details-toggle')
      expect(toggle).toHaveAttribute('aria-pressed', 'true')
      expect(toggle).toHaveAccessibleName('Collapse details')
      await user.click(toggle)
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details',
        'collapsed'
      )
      expect(screen.getByTestId('details-toggle')).toHaveAccessibleName(
        'Expand details'
      )
      // Desktop toggles the column, never the sheet.
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-sheet',
        'closed'
      )
    })
  })

  describe('tablet (768–1024px)', () => {
    it('keeps the switchers in the bar but has no nav button (fixed icon rail)', () => {
      viewport.setWidth(900)
      renderHeader()
      expect(screen.getByTestId('team-switcher')).toBeInTheDocument()
      expect(screen.queryByTestId('nav-toggle')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Open navigation' })
      ).not.toBeInTheDocument()
    })

    it('opens the details sheet from the header', async () => {
      viewport.setWidth(900)
      const user = userEvent.setup()
      renderHeader({ reading: true })
      const toggle = screen.getByTestId('details-toggle')
      expect(toggle).toHaveAccessibleName('Open details')
      await user.click(toggle)
      expect(screen.getByTestId('probe')).toHaveAttribute('data-sheet', 'open')
      expect(screen.getByTestId('probe')).toHaveAttribute(
        'data-details',
        'open'
      )
    })
  })

  describe('phone (< 768px)', () => {
    it('moves the switchers, search and theme into the drawer and keeps bell + avatar', () => {
      viewport.setWidth(390)
      renderHeader()
      expect(
        screen.getByRole('button', { name: 'Open navigation' })
      ).toBeInTheDocument()
      expect(screen.queryByTestId('team-switcher')).not.toBeInTheDocument()
      expect(screen.queryByTestId('project-switcher')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Search' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Toggle theme' })
      ).not.toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Notifications' })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'User menu' })
      ).toBeInTheDocument()
    })
  })
})
