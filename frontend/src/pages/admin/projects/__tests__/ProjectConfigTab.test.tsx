import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import type {
  AdminFreshnessRule,
  AdminProjectConfig,
} from '@/services/adminService'

const svc = vi.hoisted(() => ({ getProjectConfig: vi.fn() }))
vi.mock('@/services/adminService', () => ({ adminService: svc }))

import { ProjectConfigTab } from '../detail/ProjectConfigTab'

function rule(overrides: Partial<AdminFreshnessRule> = {}): AdminFreshnessRule {
  return {
    id: 'r1',
    project_id: 'p1',
    resource_types: ['prompt'],
    mediums: [],
    threshold_days: 30,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-02-03T00:00:00Z',
    ...overrides,
  }
}

const CONFIG: AdminProjectConfig = {
  project_rules: [rule()],
  team_wide_rules: [
    rule({
      id: 'r2',
      project_id: null,
      resource_types: ['artifact'],
      threshold_days: 1,
      enabled: false,
    }),
  ],
}

function renderTab() {
  return render(
    <MemoryRouter>
      <ProjectConfigTab projectId="p1" projectName="Platform" teamId="t1" />
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

it('lists project rules and team-wide rules under separate headings', async () => {
  svc.getProjectConfig.mockResolvedValue(CONFIG)
  renderTab()

  expect(await screen.findByText('Rules for this project')).toBeInTheDocument()
  expect(svc.getProjectConfig).toHaveBeenCalledWith('p1')

  const [projectRow] = screen.getAllByTestId('project-rule')
  expect(projectRow).toHaveTextContent(
    'Prompts in Platform not accessed via any medium for 30 days'
  )
  expect(within(projectRow).getByText('Enabled')).toBeInTheDocument()
  expect(within(projectRow).getByText('30 days')).toBeInTheDocument()

  expect(
    screen.getByText('Team-wide rules that also apply')
  ).toBeInTheDocument()
  const [teamRow] = screen.getAllByTestId('team-wide-rule')
  expect(teamRow).toHaveTextContent('in any project')
  expect(within(teamRow).getByText('Disabled')).toBeInTheDocument()
  expect(within(teamRow).getByText('1 day')).toBeInTheDocument()
})

it('never shows a rule scoped to another project, even if one arrives', async () => {
  const other = rule({ id: 'r3', project_id: 'p2', threshold_days: 99 })
  svc.getProjectConfig.mockResolvedValue({
    project_rules: [...CONFIG.project_rules, other],
    team_wide_rules: [...CONFIG.team_wide_rules, other],
  })
  renderTab()

  await screen.findByText('Rules for this project')
  expect(screen.getAllByTestId('project-rule')).toHaveLength(1)
  expect(screen.getAllByTestId('team-wide-rule')).toHaveLength(1)
  expect(screen.queryByText('99 days')).not.toBeInTheDocument()
})

it('shows an empty state per list', async () => {
  svc.getProjectConfig.mockResolvedValue({
    project_rules: [],
    team_wide_rules: [],
  })
  renderTab()

  expect(
    await screen.findByText('No rules target this project.')
  ).toBeInTheDocument()
  expect(screen.getByText('No team-wide rules.')).toBeInTheDocument()
})

it('is read-only: a notice and no form controls', async () => {
  svc.getProjectConfig.mockResolvedValue(CONFIG)
  const { container } = renderTab()

  expect(
    await screen.findByText(
      'Read-only view of the freshness rules that apply to this project.'
    )
  ).toBeInTheDocument()
  const panel = within(container)
  for (const role of [
    'button',
    'switch',
    'checkbox',
    'textbox',
    'combobox',
  ] as const) {
    expect(panel.queryAllByRole(role)).toHaveLength(0)
  }
})

it('links to the team configuration', async () => {
  svc.getProjectConfig.mockResolvedValue(CONFIG)
  renderTab()

  expect(
    await screen.findByRole('link', { name: /Team configuration/ })
  ).toHaveAttribute('href', '/admin/teams/t1?tab=freshness')
})

it('shows the error state', async () => {
  svc.getProjectConfig.mockRejectedValue(new Error('config down'))
  renderTab()

  expect(
    await screen.findByText('Failed to load project configuration')
  ).toBeInTheDocument()
  expect(screen.getByText('config down')).toBeInTheDocument()
})

it('shows a skeleton while loading', () => {
  svc.getProjectConfig.mockReturnValue(new Promise(() => undefined))
  renderTab()

  expect(screen.getByTestId('config-loading')).toBeInTheDocument()
})
