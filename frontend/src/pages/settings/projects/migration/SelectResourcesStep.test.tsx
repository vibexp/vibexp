import { fireEvent, render, screen } from '@testing-library/react'

import type {
  MigrationInventory,
  MigrationResources,
} from '@/types/projectMigration'

import { SelectResourcesStep } from './SelectResourcesStep'

const mockInventory: MigrationInventory = {
  prompts: {
    count: 2,
    items: [
      { id: 'p1', name: 'Prompt A' },
      { id: 'p2', name: 'Prompt B' },
    ],
  },
  artifacts: {
    count: 1,
    items: [{ id: 'a1', name: 'Artifact A' }],
  },
  blueprints: {
    count: 0,
    items: [],
  },
  feed_items: {
    count: 2,
    items: [
      { id: 'f1', name: 'Feed Item 1' },
      { id: 'f2', name: 'Feed Item 2' },
    ],
  },
}

const emptySelection: MigrationResources = {
  prompts: { all: false, ids: [] },
  artifacts: { all: false, ids: [] },
  blueprints: { all: false, ids: [] },
  feed_items: { all: false, ids: [] },
}

function renderStep(
  selectedResources: MigrationResources = emptySelection,
  onResourcesChange = jest.fn(),
  onBack = jest.fn(),
  onNext = jest.fn()
) {
  return render(
    <SelectResourcesStep
      inventory={mockInventory}
      selectedResources={selectedResources}
      onResourcesChange={onResourcesChange}
      onBack={onBack}
      onNext={onNext}
    />
  )
}

describe('SelectResourcesStep', () => {
  it('renders all resource type accordion sections', () => {
    renderStep()

    expect(screen.getByText('Prompts')).toBeInTheDocument()
    expect(screen.getByText('Artifacts')).toBeInTheDocument()
    expect(screen.getByText('Blueprints')).toBeInTheDocument()
    expect(screen.getByText('Feed Items')).toBeInTheDocument()
  })

  it('shows count badges with resource totals', () => {
    renderStep()

    // Each section shows "selected / total" — the badge renders as a single text node
    // We check by regex to handle how text may be split in the DOM
    const badges = screen.getAllByText((_, el) => {
      return (
        el?.textContent?.trim() === '0 / 2' ||
        el?.textContent?.trim() === '0 / 1' ||
        el?.textContent?.trim() === '0 / 0'
      )
    })
    expect(badges.length).toBeGreaterThanOrEqual(3)
  })

  it('disables Next button when no resources are selected', () => {
    renderStep()

    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('enables Next button when resources are selected', () => {
    renderStep({
      ...emptySelection,
      prompts: { all: false, ids: ['p1'] },
    })

    expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled()
  })

  it('shows "No resources selected" message when selection is empty', () => {
    renderStep()

    expect(screen.getByText(/no resources selected/i)).toBeInTheDocument()
  })

  it('shows selected count when resources are selected', () => {
    renderStep({
      ...emptySelection,
      prompts: { all: false, ids: ['p1', 'p2'] },
      artifacts: { all: false, ids: ['a1'] },
    })

    expect(screen.getByText(/3 resources selected/i)).toBeInTheDocument()
  })

  it('shows singular form for single resource', () => {
    renderStep({
      ...emptySelection,
      prompts: { all: false, ids: ['p1'] },
    })

    expect(screen.getByText(/1 resource selected/i)).toBeInTheDocument()
  })

  it('calls onBack when Back button is clicked', () => {
    const onBack = jest.fn()
    renderStep(emptySelection, jest.fn(), onBack)

    fireEvent.click(screen.getByRole('button', { name: /back/i }))

    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('calls onNext when Next button is clicked with selection', () => {
    const onNext = jest.fn()
    renderStep(
      { ...emptySelection, prompts: { all: false, ids: ['p1'] } },
      jest.fn(),
      jest.fn(),
      onNext
    )

    fireEvent.click(screen.getByRole('button', { name: /next/i }))

    expect(onNext).toHaveBeenCalledTimes(1)
  })

  it('shows "No prompts" message for empty resource type', () => {
    renderStep()

    expect(
      screen.getByText(/no blueprints in this project/i)
    ).toBeInTheDocument()
  })

  it('counts all:true as full inventory count', () => {
    renderStep({
      ...emptySelection,
      prompts: { all: true, ids: ['p1', 'p2'] },
    })

    expect(screen.getByText(/2 resources selected/i)).toBeInTheDocument()
  })
})
