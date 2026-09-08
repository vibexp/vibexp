import { render, screen, within } from '@testing-library/react'

import {
  expectNoCardChrome,
  FlatSurface,
} from '@/lib/testing/panelPresentation'

import {
  type ResourceKindKey,
  resourceRegistry,
  ResourceTaxonomySection,
} from '..'

/*
 * Like the metadata section, this is a generator: the guarantee is that every
 * kind's grouping fields land in one block from its descriptor, whatever shape
 * they happen to have. So the fixtures are real registry descriptors — a
 * descriptor change surfaces here — and the assertions are on the group labels
 * and chips, not on any one page's markup.
 */

const HEADING = 'Labels & metadata'

function renderSection(
  kind: ResourceKindKey,
  resource: Record<string, unknown>
) {
  return render(
    <ResourceTaxonomySection
      descriptor={resourceRegistry[kind]}
      resource={resource}
    />
  )
}

function section() {
  return screen.getByTestId('taxonomy-section')
}

describe('ResourceTaxonomySection', () => {
  describe('list-shaped fields only', () => {
    it('renders prompt labels as chips under the field label', () => {
      renderSection('prompt', {
        name: 'Code review',
        labels: ['code-review', 'documentation'],
      })
      expect(screen.getByRole('heading', { name: HEADING })).toBeInTheDocument()
      expect(screen.getByText('Labels')).toBeInTheDocument()
      expect(screen.getByText('code-review')).toBeInTheDocument()
      expect(screen.getByText('documentation')).toBeInTheDocument()
    })

    it('renders a string-valued taxonomy field as a single chip', () => {
      // Blueprint `subtype` is declared `taxonomy` and rendered nowhere before
      // #904 — a bare string, not a list.
      renderSection('blueprint', { title: 'Claude rules', subtype: 'rules' })
      expect(screen.getByText('Subtype')).toBeInTheDocument()
      expect(screen.getByText('rules')).toBeInTheDocument()
    })

    it('drops non-string entries rather than rendering them', () => {
      renderSection('prompt', { labels: ['keep', 42, null, ''] })
      expect(screen.getByText('keep')).toBeInTheDocument()
      expect(screen.queryByText('42')).not.toBeInTheDocument()
    })
  })

  describe('pair-shaped fields only', () => {
    it('renders the artifact metadata bag as key/value pairs', () => {
      renderSection('artifact', {
        title: 'Weekly report',
        metadata: { author: 'ada', active_count: 5 },
      })
      expect(screen.getByText('Author')).toBeInTheDocument()
      expect(screen.getByText('ada')).toBeInTheDocument()
      expect(screen.getByText('Active count')).toBeInTheDocument()
    })

    it('never claims a scalar meta field the metadata rows already render', () => {
      // `mcp_expose` / `is_shared` / `project_id` are `meta` but scalar, so
      // they belong to #903's rows. Classifying by role instead of by value
      // shape would double-render them here.
      renderSection('prompt', {
        labels: ['keep'],
        mcp_expose: true,
        is_shared: true,
        project_id: 'p1',
      })
      expect(screen.queryByText('MCP')).not.toBeInTheDocument()
      expect(screen.queryByText('Shared')).not.toBeInTheDocument()
      expect(screen.queryByText('p1')).not.toBeInTheDocument()
    })

    it('leaves a rendered meta field to the metadata rows', () => {
      // `path` carries its own `render`, so it is a row by construction even
      // if a payload ever hands it an object.
      renderSection('blueprint', { path: { nested: true } })
      expect(screen.queryByText('Path')).not.toBeInTheDocument()
    })
  })

  describe('both shapes', () => {
    it('lifts memory metadata.tags to chips and leaves the rest as pairs', () => {
      renderSection('memory', {
        id: 'mem-1',
        metadata: { tags: ['alpha', 'beta'], type: 'note' },
      })
      const block = section()
      expect(within(block).getByText('Tags')).toBeInTheDocument()
      expect(within(block).getByText('alpha')).toBeInTheDocument()
      expect(within(block).getByText('beta')).toBeInTheDocument()
      // The non-`tags` keys stay pairs — the split survives until #910.
      expect(within(block).getByText('Type')).toBeInTheDocument()
      expect(within(block).getByText('note')).toBeInTheDocument()
    })

    it("lifts a tags key out of any pair bag, not just memory's", () => {
      renderSection('artifact', {
        metadata: { tags: ['alpha'], other: 'kept' },
      })
      const block = section()
      expect(within(block).getByText('Tags')).toBeInTheDocument()
      expect(within(block).getByText('alpha')).toBeInTheDocument()
      expect(within(block).getByText('Other')).toBeInTheDocument()
      expect(within(block).getByText('kept')).toBeInTheDocument()
    })

    it.each([
      ['a number', 42, '42'],
      ['a mixed array', ['alpha', 42], '["alpha",42]'],
    ])(
      'keeps a tags value the chip row cannot represent (%s) as a pair',
      (_name, tags, rendered) => {
        // The bag used to round-trip every key through `MetaValue`; the lift is
        // a display choice and must never drop data.
        renderSection('artifact', { metadata: { tags } })
        const block = section()
        expect(within(block).getByText('Tags')).toBeInTheDocument()
        expect(within(block).getByText(rendered)).toBeInTheDocument()
      }
    )

    it('renders a repeated tag once', () => {
      renderSection('memory', { metadata: { tags: ['alpha', 'alpha'] } })
      expect(screen.getAllByText('alpha')).toHaveLength(1)
    })

    it('renders declared taxonomy and the metadata bag under one heading', () => {
      renderSection('blueprint', {
        subtype: 'rules',
        metadata: { origin: 'import' },
      })
      expect(screen.getAllByRole('heading', { name: HEADING })).toHaveLength(1)
      const block = section()
      expect(within(block).getByText('rules')).toBeInTheDocument()
      expect(within(block).getByText('import')).toBeInTheDocument()
    })

    it('keeps groups in descriptor field order', () => {
      // gallery-prompt declares `category` before `tags`.
      renderSection('gallery-prompt', {
        category: 'writing',
        tags: ['sharp'],
      })
      const block = section()
      const labels = Array.from(
        block.querySelectorAll('span.text-muted-foreground')
      ).map(node => node.textContent)
      expect(labels).toEqual(['Category', 'Tags'])
    })
  })

  describe('all empty', () => {
    it.each([
      ['prompt', { name: 'Code review', labels: null }],
      ['prompt', { name: 'Code review', labels: [] }],
      ['artifact', { title: 'Weekly report' }],
      ['artifact', { title: 'Weekly report', metadata: {} }],
      ['memory', { id: 'mem-1', metadata: { tags: [] } }],
      ['blueprint', { title: 'Claude rules', subtype: '' }],
    ] as [ResourceKindKey, Record<string, unknown>][])(
      'renders no node at all for %s with nothing to group by',
      (kind, resource) => {
        const { container } = renderSection(kind, resource)
        expect(container.firstChild).toBeNull()
        expect(
          screen.queryByRole('heading', { name: HEADING })
        ).not.toBeInTheDocument()
      }
    )
  })

  it('paints no card chrome inside the details column', () => {
    const { container } = render(
      <FlatSurface>
        <ResourceTaxonomySection
          descriptor={resourceRegistry.prompt}
          resource={{ labels: ['alpha'] }}
        />
      </FlatSurface>
    )
    expectNoCardChrome(container.firstChild as HTMLElement)
    expect(screen.getByRole('heading', { name: HEADING })).toHaveClass(
      'text-sm'
    )
  })
})
