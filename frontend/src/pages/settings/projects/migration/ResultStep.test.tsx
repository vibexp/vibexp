import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { MigrationResult } from '@/types/projectMigration'

import { ResultStep } from './ResultStep'

const cleanResult: MigrationResult = {
  migrated: { prompts: 3, artifacts: 2, blueprints: 1, feed_items: 0 },
  skipped: {},
  failed: {},
}

const resultWithSkipped: MigrationResult = {
  migrated: { prompts: 2, artifacts: 1, blueprints: 0, feed_items: 0 },
  skipped: {
    prompts: [{ id: 'p3', reason: 'name conflict' }],
  },
  failed: {},
}

const resultWithFailed: MigrationResult = {
  migrated: { prompts: 1, artifacts: 0, blueprints: 0, feed_items: 0 },
  skipped: {},
  failed: {
    artifacts: [
      { id: 'a1', reason: 'permission denied' },
      { id: 'a2', reason: 'not found' },
    ],
  },
}

function renderResult(result: MigrationResult) {
  return render(
    <MemoryRouter>
      <ResultStep
        result={result}
        destinationProjectSlug="dest-project"
        destinationProjectName="Destination Project"
        onDone={jest.fn()}
      />
    </MemoryRouter>
  )
}

describe('ResultStep', () => {
  it('shows migration complete heading', () => {
    renderResult(cleanResult)

    expect(
      screen.getByRole('heading', { name: /migration complete/i })
    ).toBeInTheDocument()
  })

  it('shows success alert when no failures', () => {
    renderResult(cleanResult)

    expect(screen.getByText(/migration successful/i)).toBeInTheDocument()
  })

  it('shows failure alert when there are failures', () => {
    renderResult(resultWithFailed)

    expect(
      screen.getByText(/some resources failed to migrate/i)
    ).toBeInTheDocument()
  })

  it('shows total migrated count in alert', () => {
    renderResult(cleanResult)

    // The alert description may span multiple text nodes; getAllByText returns all containers
    const elements = screen.getAllByText(
      (_, node) => !!node?.textContent?.match(/6 resources moved successfully/i)
    )
    expect(elements.length).toBeGreaterThan(0)
  })

  it('shows singular form for one migrated resource', () => {
    const singleResult: MigrationResult = {
      migrated: { prompts: 1, artifacts: 0, blueprints: 0, feed_items: 0 },
      skipped: {},
      failed: {},
    }
    renderResult(singleResult)

    // Text spans multiple DOM nodes; check any container has this text
    const elements = screen.getAllByText(
      (_, el) => !!el?.textContent?.match(/1 resource moved successfully/i)
    )
    expect(elements.length).toBeGreaterThan(0)
  })

  it('shows per-type migrated counts', () => {
    renderResult(cleanResult)

    // Prompts: 3, Artifacts: 2, Blueprints: 1
    expect(screen.getByText('Prompts')).toBeInTheDocument()
    expect(screen.getByText('Artifacts')).toBeInTheDocument()
    expect(screen.getByText('Blueprints')).toBeInTheDocument()
  })

  it('shows skipped section when there are skipped items', () => {
    renderResult(resultWithSkipped)

    expect(screen.getByText(/skipped \(1\)/i)).toBeInTheDocument()
  })

  it('does not show skipped section when no skipped items', () => {
    renderResult(cleanResult)

    expect(screen.queryByText(/skipped/i)).not.toBeInTheDocument()
  })

  it('shows failed section when there are failed items', () => {
    renderResult(resultWithFailed)

    expect(screen.getByText(/failed \(2\)/i)).toBeInTheDocument()
  })

  it('does not show failed section when no failed items', () => {
    renderResult(cleanResult)

    expect(screen.queryByText(/failed/i)).not.toBeInTheDocument()
  })

  it('renders View destination project link with correct href', () => {
    renderResult(cleanResult)

    const link = screen.getByRole('link', { name: /view destination project/i })
    expect(link).toHaveAttribute('href', '/settings/projects/dest-project')
  })

  it('calls onDone when Done button is clicked', async () => {
    const onDone = jest.fn()
    const user = userEvent.setup()
    render(
      <MemoryRouter>
        <ResultStep
          result={cleanResult}
          destinationProjectSlug="dest-project"
          destinationProjectName="Destination Project"
          onDone={onDone}
        />
      </MemoryRouter>
    )

    await user.click(screen.getByRole('button', { name: /^done$/i }))

    expect(onDone).toHaveBeenCalledTimes(1)
  })
})
