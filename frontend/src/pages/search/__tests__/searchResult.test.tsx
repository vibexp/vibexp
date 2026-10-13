import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import type { SearchResultItem } from '@/services/searchService'

import { SearchResultCard } from '../searchResult'

const item = {
  id: 'p1',
  chunk_id: 'c1',
  type: 'prompt',
  title: 'Deploy checklist',
  slug: 'deploy-checklist',
  excerpt:
    '## Before you **deploy**\nRun `make test` and read [the guide](https://example.com).',
  project_name: '',
} as unknown as SearchResultItem

function renderCard(query: string) {
  render(
    <MemoryRouter>
      <SearchResultCard
        item={item}
        query={query}
        expanded={false}
        onToggleExpand={vi.fn()}
      />
    </MemoryRouter>
  )
}

describe('SearchResultCard', () => {
  it('shows the excerpt as plain text, without its markdown syntax', () => {
    renderCard('')

    expect(
      screen.getByText('Before you deploy Run make test and read the guide.')
    ).toBeInTheDocument()
  })

  it('marks the query terms in the title and the excerpt', () => {
    const { container } = render(
      <MemoryRouter>
        <SearchResultCard
          item={item}
          query="deploy"
          expanded={false}
          onToggleExpand={vi.fn()}
        />
      </MemoryRouter>
    )

    const marks = [...container.querySelectorAll('mark')].map(
      mark => mark.textContent
    )
    expect(marks).toEqual(['Deploy', 'deploy'])
  })
})
