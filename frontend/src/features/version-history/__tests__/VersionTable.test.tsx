import '@testing-library/jest-dom'

import { render, screen } from '@testing-library/react'

import { TABLE_HEAD_CLASS } from '@/components/patterns/list-page'

import { VersionTable } from '../VersionTable'

const HEAD_CLASSES = TABLE_HEAD_CLASS.split(' ')

const noop = () => undefined

function renderTable() {
  render(
    <VersionTable
      entries={[]}
      hasSnapshots
      selected={[]}
      onToggleSort={noop}
      onToggleSelect={noop}
      onView={noop}
      onRestore={noop}
    />
  )
}

describe('VersionTable header', () => {
  it('gives every head cell the shared list-table header style', () => {
    renderTable()

    const headCells = screen.getAllByRole('columnheader')
    expect(headCells).toHaveLength(7)
    for (const cell of headCells) {
      // Read from TABLE_HEAD_CLASS itself, so this pins "the version-history
      // table and the list tables share ONE header source" (#909) rather than
      // two copies that happen to match today.
      expect(cell).toHaveClass(...HEAD_CLASSES)
    }
  })

  it('keeps the sort affordance on the sortable columns only', () => {
    renderTable()

    const sortable = screen
      .getAllByRole('columnheader')
      .filter(cell => cell.classList.contains('vhc-sort'))
    expect(sortable.map(cell => cell.textContent)).toEqual(['Version', 'When'])
  })
})
