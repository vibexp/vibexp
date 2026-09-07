import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'

import {
  type ResourceKindKey,
  ResourceMetadataSection,
  resourceRegistry,
} from '..'

/*
 * The section is a generator: what it must guarantee is that every kind
 * produces the same rows in the same order from its descriptor, not that any
 * one page happens to look right. So the assertions are on the row LABELS in
 * order — the thing that used to differ four ways.
 */

const PROJECT = { name: 'Design System', slug: 'design-system' }
const href = (p: { slug: string }) => `/teams/t1/projects/${p.slug}/edit`

function rowLabels() {
  return screen
    .getAllByRole('listitem')
    .map(li => li.firstElementChild?.textContent ?? '')
}

function renderSection(
  kind: ResourceKindKey,
  resource: Record<string, unknown>,
  extra: Partial<React.ComponentProps<typeof ResourceMetadataSection>> = {}
) {
  return render(
    <MemoryRouter>
      <ResourceMetadataSection
        descriptor={resourceRegistry[kind]}
        resource={resource}
        project={PROJECT}
        projectHref={href}
        {...extra}
      />
    </MemoryRouter>
  )
}

const PROMPT = {
  name: 'Code review',
  slug: 'code-review',
  status: 'published',
  mcp_expose: true,
  is_shared: false,
  project_id: 'p1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

const ARTIFACT = {
  title: 'Weekly report',
  slug: 'weekly-report',
  type: 'work_reports',
  status: 'draft',
  project_id: 'p1',
  metadata: { anything: 'goes' },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

const BLUEPRINT = {
  title: 'Claude rules',
  slug: 'claude-rules',
  type: 'claude-code',
  status: 'active',
  path: 'CLAUDE.md',
  project_id: 'p1',
  source: {
    repo: 'https://github.com/vibexp/vibexp',
    commit_sha: 'abcdef1234567890',
    imported_at: '2026-01-01T00:00:00Z',
  },
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

const MEMORY = {
  id: 'mem-1',
  text: 'Remember this',
  status: 'active',
  project_id: 'p1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

describe('ResourceMetadataSection', () => {
  it('keeps the panel test id the pages and e2e specs address', () => {
    renderSection('prompt', PROMPT)
    expect(screen.getByTestId('metadata-panel')).toBeInTheDocument()
  })

  it('generates the prompt rows in the fixed order', () => {
    renderSection('prompt', PROMPT)
    expect(rowLabels()).toEqual([
      'Status',
      'Slug',
      'Project',
      'MCP',
      'Shared',
      'Created',
      'Updated',
    ])
    expect(screen.getByText('Exposed')).toBeInTheDocument()
    expect(screen.getByText('Not shared')).toBeInTheDocument()
  })

  it('generates the artifact rows in the same order, Type first', () => {
    renderSection('artifact', ARTIFACT)
    expect(rowLabels()).toEqual([
      'Type',
      'Status',
      'Slug',
      'Project',
      'Created',
      'Updated',
    ])
    // `valueLabels` turns the wire value into the badge text.
    expect(screen.getByText('Work reports')).toBeInTheDocument()
    expect(screen.getByText('Draft')).toBeInTheDocument()
    // The `metadata` blob belongs to AdditionalDataCard, not to a row.
    expect(rowLabels()).not.toContain('Metadata')
  })

  it('expands the blueprint provenance object into one row per field', () => {
    renderSection('blueprint', BLUEPRINT)
    expect(rowLabels()).toEqual([
      'Type',
      'Status',
      'Slug',
      'Project',
      'Path',
      'Source',
      'Commit',
      'Imported',
      'Created',
      'Updated',
    ])
    expect(screen.getByText('github.com/vibexp/vibexp')).toBeInTheDocument()
    expect(screen.getByText('abcdef1')).toBeInTheDocument()
  })

  it('generates the memory rows, including the Project row it alone used to have', () => {
    renderSection('memory', MEMORY)
    expect(rowLabels()).toEqual([
      'Status',
      'ID',
      'Project',
      'Created',
      'Updated',
    ])
  })

  it('renders no row for a field absent from the payload', () => {
    const source = { ...BLUEPRINT.source, repo: undefined }
    renderSection('blueprint', { ...BLUEPRINT, source })
    expect(rowLabels()).not.toContain('Source')
    expect(rowLabels()).toContain('Commit')
  })

  it('omits the Project row until the project resolves', () => {
    renderSection('memory', MEMORY, { project: null })
    expect(rowLabels()).not.toContain('Project')
  })

  it('links the Project row through the caller-supplied href', () => {
    renderSection('memory', MEMORY)
    expect(screen.getByRole('link', { name: /Design System/ })).toHaveAttribute(
      'href',
      '/teams/t1/projects/design-system/edit'
    )
  })

  it('forwards version history to the panel', () => {
    renderSection('artifact', ARTIFACT, {
      versionHistory: { count: 3, to: '/artifacts/p1/weekly-report/versions' },
    })
    expect(screen.getByTestId('metadata-version-history-link')).toHaveAttribute(
      'href',
      '/artifacts/p1/weekly-report/versions'
    )
  })
})
