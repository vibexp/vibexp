import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'

import { Connect } from '../Connect'
import { CONNECT_PATHS, CONNECT_TABS, isConnectTab } from '../connectTabs'

// Each tab is an existing page with its own suite; here they are stand-ins so
// the test is about the tab bar and the routing.
vi.mock('@/pages/mcp/VibeXPMCP', () => ({
  VibeXPMCP: () => <h1>MCP page</h1>,
}))
vi.mock('@/pages/integrations/CLI', () => ({
  CliPage: () => <h1>CLI page</h1>,
}))
vi.mock('@/pages/integrations/RestApis', () => ({
  RestApis: () => <h1>REST page</h1>,
}))
vi.mock('@/pages/integrations/ApiClient', () => ({
  ApiClient: () => <h1>Client page</h1>,
}))

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Where />
      <Routes>
        <Route path="connect" element={<Connect />} />
        <Route path="connect/:tab" element={<Connect />} />
      </Routes>
    </MemoryRouter>
  )
}

describe('Connect', () => {
  it('offers the four tabs in order, as links to their own URLs', () => {
    renderAt('/connect/mcp')

    const links = screen
      .getByRole('navigation', { name: 'Connect' })
      .querySelectorAll('a')
    expect(
      [...links].map(a => [a.textContent, a.getAttribute('href')])
    ).toEqual([
      ['MCP', '/connect/mcp'],
      ['CLI', '/connect/cli'],
      ['REST API', '/connect/rest-api'],
      ['API client', '/connect/api-client'],
    ])
  })

  it.each([
    ['/connect/mcp', 'MCP page', 'MCP'],
    ['/connect/cli', 'CLI page', 'CLI'],
    ['/connect/rest-api', 'REST page', 'REST API'],
    ['/connect/api-client', 'Client page', 'API client'],
  ])('shows the tab named in the URL: %s', (path, heading, tab) => {
    renderAt(path)

    // The tab's own page, and with it the page's single <h1>.
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(
      screen.getByRole('heading', { level: 1, name: heading })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: tab })).toHaveAttribute(
      'aria-current',
      'page'
    )
  })

  it.each([['/connect'], ['/connect/nonsense']])(
    'lands %s on the MCP tab, the default',
    path => {
      renderAt(path)

      expect(screen.getByTestId('where')).toHaveTextContent('/connect/mcp')
      expect(
        screen.getByRole('heading', { level: 1, name: 'MCP page' })
      ).toBeInTheDocument()
    }
  )
})

describe('connectTabs', () => {
  it('names a path for every tab', () => {
    expect(Object.values(CONNECT_PATHS)).toEqual(
      CONNECT_TABS.map(tab => `/connect/${tab.slug}`)
    )
  })

  it('recognises only its own slugs', () => {
    expect(isConnectTab('cli')).toBe(true)
    expect(isConnectTab('rest-apis')).toBe(false)
    expect(isConnectTab(undefined)).toBe(false)
  })
})
