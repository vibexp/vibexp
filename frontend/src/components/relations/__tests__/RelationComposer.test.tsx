import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import { RelationComposer } from '@/components/relations/RelationComposer'

const showError = jest.fn()
jest.mock('@/hooks', () => ({
  useAlerts: () => ({ showError }),
}))

const getArtifacts = jest.fn()
const getBlueprints = jest.fn()
const getPrompts = jest.fn()
const getMemories = jest.fn()
jest.mock('@/services/artifactService', () => ({
  artifactService: { getArtifacts: (...a: unknown[]) => getArtifacts(...a) },
}))
jest.mock('@/services/blueprintService', () => ({
  blueprintService: { getBlueprints: (...a: unknown[]) => getBlueprints(...a) },
}))
jest.mock('@/services/promptService', () => ({
  promptService: { getPrompts: (...a: unknown[]) => getPrompts(...a) },
}))
jest.mock('@/services/memoryService', () => ({
  memoryService: { getMemories: (...a: unknown[]) => getMemories(...a) },
}))

function renderComposer(
  overrides: Partial<React.ComponentProps<typeof RelationComposer>> = {}
) {
  const props = {
    teamId: 'team-1',
    subjectType: 'artifact' as const,
    subjectId: 'a1',
    onAdd: jest.fn().mockResolvedValue(undefined),
    onSuccess: jest.fn(),
    onCancel: jest.fn(),
    ...overrides,
  }
  render(<RelationComposer {...props} />)
  return props
}

beforeEach(() => {
  jest.clearAllMocks()
  getArtifacts.mockResolvedValue({
    artifacts: [
      { id: 'a1', title: 'Self artifact' },
      { id: 'a2', title: 'Other artifact' },
    ],
  })
  getBlueprints.mockResolvedValue({
    blueprints: [{ id: 'b1', title: 'Go standards' }],
  })
  getPrompts.mockResolvedValue({ prompts: [{ id: 'p1', name: 'Summarizer' }] })
  getMemories.mockResolvedValue({
    memories: [{ id: 'm1', text: 'The team prefers X' }],
  })
})

test('defaults to governed-by and loads blueprint targets', async () => {
  renderComposer()
  expect(await screen.findByText('Go standards')).toBeInTheDocument()
  expect(getBlueprints).toHaveBeenCalledWith('team-1', {})
  // Only blueprints are offered for governed-by (matrix constraint).
  expect(getPrompts).not.toHaveBeenCalled()
})

test('changing the relation type reloads matrix-constrained targets', async () => {
  renderComposer()
  await screen.findByText('Go standards')

  // built-from -> prompts
  fireEvent.change(screen.getByTestId('relation-type-select'), {
    target: { value: 'built-from' },
  })
  expect(await screen.findByText('Summarizer')).toBeInTheDocument()

  // explained-by -> memories
  fireEvent.change(screen.getByTestId('relation-type-select'), {
    target: { value: 'explained-by' },
  })
  expect(await screen.findByText('The team prefers X')).toBeInTheDocument()
})

test('supersedes targets the subject type and excludes the subject itself', async () => {
  renderComposer({ subjectType: 'artifact', subjectId: 'a1' })
  fireEvent.change(screen.getByTestId('relation-type-select'), {
    target: { value: 'supersedes' },
  })
  // a2 is offered; a1 (the subject) is filtered out.
  expect(await screen.findByText('Other artifact')).toBeInTheDocument()
  expect(screen.queryByText('Self artifact')).not.toBeInTheDocument()
})

test('submitting a chosen target calls onAdd then onSuccess', async () => {
  const props = renderComposer()
  await screen.findByText('Go standards')

  fireEvent.change(screen.getByTestId('relation-target-select'), {
    target: { value: 'b1' },
  })
  fireEvent.click(screen.getByTestId('relation-add-submit'))

  await waitFor(() => {
    expect(props.onAdd).toHaveBeenCalledWith('governed-by', 'blueprint', 'b1')
  })
  await waitFor(() => {
    expect(props.onSuccess).toHaveBeenCalled()
  })
})

test('an onAdd failure surfaces an alert and does not call onSuccess', async () => {
  const onAdd = jest.fn().mockRejectedValue(new Error('nope'))
  const props = renderComposer({ onAdd })
  await screen.findByText('Go standards')

  fireEvent.change(screen.getByTestId('relation-target-select'), {
    target: { value: 'b1' },
  })
  fireEvent.click(screen.getByTestId('relation-add-submit'))

  await waitFor(() => {
    expect(showError).toHaveBeenCalled()
  })
  expect(props.onSuccess).not.toHaveBeenCalled()
})

test('a target-load failure leaves an empty picker', async () => {
  getBlueprints.mockRejectedValue(new Error('down'))
  renderComposer()
  await waitFor(() => {
    expect(getBlueprints).toHaveBeenCalled()
  })
  expect(screen.queryByText('Go standards')).not.toBeInTheDocument()
})

test('cancel invokes onCancel', () => {
  const props = renderComposer()
  fireEvent.click(screen.getByText('Cancel'))
  expect(props.onCancel).toHaveBeenCalled()
})
