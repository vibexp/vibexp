import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import { RelationsPanel } from '@/components/relations/RelationsPanel'
import type { UseRelationsResult } from '@/hooks/useRelations'
import type { RelatedResource } from '@/services/relationService'

let mockState: UseRelationsResult
jest.mock('@/hooks/useRelations', () => ({
  useRelations: () => mockState,
}))

// Stub the composer — its own suite exercises the matrix picker; here we only
// assert the panel opens it, without pulling in the four resource services.
jest.mock('@/components/relations/RelationComposer', () => ({
  RelationComposer: () => <div data-testid="relation-composer-stub" />,
}))

const grantedPerms = new Set<string>()
let canDismissRet = false
jest.mock('@/hooks/usePermissions', () => ({
  usePermissions: () => ({
    can: (p: string) => grantedPerms.has(p),
    canDeleteResource: () => canDismissRet,
  }),
}))

const showError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showError }),
}))

function makeRelation(
  overrides: Partial<RelatedResource> = {}
): RelatedResource {
  return {
    relation_id: 'r1',
    relation_type: 'governed-by',
    direction: 'outgoing',
    origin: 'ai',
    status: 'suggested',
    resource_type: 'blueprint',
    resource_id: 'b1',
    title: 'Go standards',
    slug: 'go-standards',
    project_id: 'p1',
    created_at: '2026-07-21T09:00:00Z',
    ...overrides,
  }
}

function setState(overrides: Partial<UseRelationsResult> = {}) {
  mockState = {
    relations: [],
    loading: false,
    error: false,
    reload: jest.fn(),
    addRelation: jest.fn().mockResolvedValue(undefined),
    confirmRelation: jest.fn().mockResolvedValue(undefined),
    removeRelation: jest.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

function renderPanel() {
  return render(
    <MemoryRouter>
      <RelationsPanel teamId="team-1" resourceType="artifact" resourceId="a1" />
    </MemoryRouter>
  )
}

beforeEach(() => {
  grantedPerms.clear()
  canDismissRet = false
  showError.mockClear()
  setState()
})

test('empty state when there are no relations', () => {
  renderPanel()
  expect(screen.getByText(/no relations yet/i)).toBeInTheDocument()
})

test('renders a row with the direction label and a link to the target', () => {
  setState({ relations: [makeRelation()] })
  renderPanel()
  expect(screen.getByText('governed by')).toBeInTheDocument()
  const link = screen.getByTestId('relation-target-link')
  expect(link).toHaveTextContent('Go standards')
  expect(link).toHaveAttribute('href', '/blueprints/p1/go-standards')
})

test('a suggested ai edge shows the provenance badge', () => {
  setState({ relations: [makeRelation({ status: 'suggested', origin: 'ai' })] })
  renderPanel()
  expect(screen.getByTestId('relation-suggested-badge')).toBeInTheDocument()
})

test('Add button appears only with resource.create', () => {
  renderPanel()
  expect(screen.queryByTestId('relation-add-button')).not.toBeInTheDocument()
  grantedPerms.add('resource.create')
  renderPanel()
  expect(screen.getByTestId('relation-add-button')).toBeInTheDocument()
})

test('Accept confirms; gated on resource.update.any', () => {
  grantedPerms.add('resource.update.any')
  const confirmRelation = jest.fn().mockResolvedValue(undefined)
  setState({
    relations: [makeRelation({ status: 'suggested' })],
    confirmRelation,
  })
  renderPanel()
  fireEvent.click(screen.getByTestId('relation-accept'))
  expect(confirmRelation).toHaveBeenCalledWith('r1')
})

test('Dismiss deletes; gated on canDeleteResource', () => {
  canDismissRet = true
  const removeRelation = jest.fn().mockResolvedValue(undefined)
  setState({
    relations: [makeRelation({ status: 'suggested' })],
    removeRelation,
  })
  renderPanel()
  fireEvent.click(screen.getByTestId('relation-dismiss'))
  expect(removeRelation).toHaveBeenCalledWith('r1')
})

test('confirmed edges show no Accept/Dismiss', () => {
  grantedPerms.add('resource.update.any')
  canDismissRet = true
  setState({
    relations: [makeRelation({ status: 'confirmed', origin: 'human' })],
  })
  renderPanel()
  expect(screen.queryByTestId('relation-accept')).not.toBeInTheDocument()
  expect(screen.queryByTestId('relation-dismiss')).not.toBeInTheDocument()
  expect(
    screen.queryByTestId('relation-suggested-badge')
  ).not.toBeInTheDocument()
})

test('loading shows the skeleton', () => {
  setState({ loading: true })
  renderPanel()
  expect(screen.getByTestId('relations-loading')).toBeInTheDocument()
})

test('error shows a retry that reloads', () => {
  const reload = jest.fn()
  setState({ error: true, reload })
  renderPanel()
  fireEvent.click(screen.getByText('Retry'))
  expect(reload).toHaveBeenCalled()
})

test('clicking Add opens the composer', () => {
  grantedPerms.add('resource.create')
  renderPanel()
  expect(screen.queryByTestId('relation-composer-stub')).not.toBeInTheDocument()
  fireEvent.click(screen.getByTestId('relation-add-button'))
  expect(screen.getByTestId('relation-composer-stub')).toBeInTheDocument()
})

test('a failed confirm surfaces an alert', async () => {
  grantedPerms.add('resource.update.any')
  const confirmRelation = jest.fn().mockRejectedValue(new Error('boom'))
  setState({
    relations: [makeRelation({ status: 'suggested' })],
    confirmRelation,
  })
  renderPanel()
  fireEvent.click(screen.getByTestId('relation-accept'))
  await waitFor(() => {
    expect(showError).toHaveBeenCalled()
  })
})

test('a failed dismiss surfaces an alert', async () => {
  canDismissRet = true
  const removeRelation = jest.fn().mockRejectedValue(new Error('nope'))
  setState({
    relations: [makeRelation({ status: 'suggested' })],
    removeRelation,
  })
  renderPanel()
  fireEvent.click(screen.getByTestId('relation-dismiss'))
  await waitFor(() => {
    expect(showError).toHaveBeenCalled()
  })
})
