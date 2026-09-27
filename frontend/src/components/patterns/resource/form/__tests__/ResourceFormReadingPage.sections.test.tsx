import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { ShellProvider } from '@/components/layout/ShellContext'
import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import {
  ResourceReadingPage,
  type ResourceRef,
} from '@/components/resource-detail/ResourceReadingPage'
import { RESOURCE_SECTION_IDS } from '@/components/resource-detail/resourceSections'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

// The picker is an async combobox over the projects API; stub it to a button
// that selects a fixed project, as the page suites do.
vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    onChange,
    id,
    titleTrigger,
    'data-testid': testId,
  }: {
    onChange: (id: string | null) => void
    id?: string
    titleTrigger?: boolean
    'data-testid'?: string
  }) => (
    <button
      type="button"
      id={id}
      data-title-trigger={titleTrigger ? 'true' : undefined}
      data-testid={testId}
      onClick={() => {
        onChange('p2')
      }}
    >
      select project
    </button>
  ),
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [
      { id: 't1', slug: 'general', name: 'General' },
      { id: 't2', slug: 'report', name: 'Report' },
    ],
    isLoading: false,
  }),
}))

// The side panels fetch on mount; the column's shape is what is under test,
// so each renders its own heading and nothing else.
vi.mock('@/components/attachments/ResourceAttachments', () => ({
  ResourceAttachments: ({ ownerId }: { ownerId: string }) => (
    <div data-testid="attachments-panel" data-owner={ownerId}>
      <h3>Attachments</h3>
    </div>
  ),
}))
vi.mock('@/components/access-activity/AccessActivityPanel', () => ({
  AccessActivityPanel: () => <h3>Access activity</h3>,
}))
vi.mock('@/components/comments/CommentsPanel', () => ({
  CommentsPanel: () => <h3>Comments</h3>,
}))
vi.mock('@/components/relations/RelationsPanel', () => ({
  RelationsPanel: () => <h3>Relations</h3>,
}))

import { MemoryTagsField } from '@/pages/memories/MemoryTagsField'
import { McpExposureSwitch } from '@/pages/prompts/editor/McpExposureSwitch'

import { artifactDescriptor } from '../../descriptors/artifact'
import { blueprintDescriptor } from '../../descriptors/blueprint'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import { ResourceMetadataSection } from '../../ResourceMetadataSection'
import { ResourceTaxonomySection } from '../../ResourceTaxonomySection'
import type { ResourceDescriptor } from '../../types'
import type { ResourceFormReadingPageProps } from '../ResourceFormReadingPage'
import { ResourceFormReadingPage } from '../ResourceFormReadingPage'

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const CREATED = '2026-01-02T03:04:05Z'
const UPDATED = '2026-03-04T05:06:07Z'

/** A saved resource of each kind, as the API returns it. */
const RECORDS: Record<string, Record<string, unknown>> = {
  artifact: {
    id: 'a1',
    title: 'Quarterly report',
    slug: 'quarterly-report',
    project_id: 'p1',
    content: 'Body',
    type: 'general',
    status: 'active',
    labels: ['finance'],
    metadata: { owner: 'ops' },
    created_at: CREATED,
    updated_at: UPDATED,
  },
  blueprint: {
    id: 'b1',
    title: 'Code review rules',
    slug: 'code-review-rules',
    project_id: 'p1',
    content: 'Body',
    type: 'rules',
    status: 'active',
    subtype: 'agents',
    labels: ['review'],
    metadata: { model: 'x' },
    created_at: CREATED,
    updated_at: UPDATED,
  },
  memory: {
    id: 'm1',
    title: 'Deploy gotcha',
    text: 'Body',
    project_id: 'p1',
    status: 'active',
    labels: ['ops'],
    metadata: { source: 'run' },
    created_at: CREATED,
    updated_at: UPDATED,
  },
  prompt: {
    id: 'pr1',
    name: 'Summarise',
    slug: 'summarise',
    description: 'Summaries',
    body: 'Body',
    project_id: 'p1',
    status: 'published',
    labels: ['writing'],
    mcp_expose: true,
    created_at: CREATED,
    updated_at: UPDATED,
  },
}

const KINDS: readonly (readonly [ResourceRef['kind'], ResourceDescriptor])[] = [
  ['artifact', artifactDescriptor],
  ['blueprint', blueprintDescriptor],
  ['memory', memoryDescriptor],
  ['prompt', promptDescriptor],
]

const VERSIONS: VersionHistoryMeta = {
  count: 3,
  to: '/somewhere/versions',
  currentVersion: 3,
  editedAt: UPDATED,
}

function refOf(kind: ResourceRef['kind']): ResourceRef {
  return { kind, id: String(RECORDS[kind].id), teamId: 'team-1' }
}

function renderEdit(
  kind: ResourceRef['kind'],
  descriptor: ResourceDescriptor,
  overrides: Partial<ResourceFormReadingPageProps> = {}
) {
  const onSubmit = vi.fn().mockResolvedValue(undefined)
  const view = render(
    <MemoryRouter>
      <ShellProvider>
        <ResourceFormReadingPage
          title={`Edit ${descriptor.singular}`}
          descriptor={descriptor}
          mode="edit"
          initialValues={RECORDS[kind]}
          resource={refOf(kind)}
          versionHistory={VERSIONS}
          onSubmit={onSubmit}
          onCancel={vi.fn()}
          {...overrides}
        />
      </ShellProvider>
    </MemoryRouter>
  )
  return { onSubmit, view }
}

/** The reading page's column, composed exactly as the four view pages do. */
function renderView(kind: ResourceRef['kind'], descriptor: ResourceDescriptor) {
  return render(
    <MemoryRouter>
      <ShellProvider>
        <ResourceReadingPage
          title="View"
          resource={refOf(kind)}
          attachments={descriptor.capabilities.attachments}
          metadata={
            <div className="space-y-5">
              <ResourceMetadataSection
                descriptor={descriptor}
                resource={RECORDS[kind]}
                versionHistory={VERSIONS}
              />
              <ResourceTaxonomySection
                descriptor={descriptor}
                resource={RECORDS[kind]}
              />
            </div>
          }
        >
          <p>Body</p>
        </ResourceReadingPage>
      </ShellProvider>
    </MemoryRouter>
  )
}

/** The column's sections and headings, in document order. */
function columnShape() {
  const column = screen.getByTestId('details-column')
  return {
    sections: [...column.querySelectorAll('section[data-section]')].map(
      section => section.getAttribute('data-section')
    ),
    headings: [...column.querySelectorAll('h3')].map(h => h.textContent),
  }
}

function metadataPanel() {
  return within(screen.getByTestId('metadata-panel'))
}

async function flush() {
  await act(async () => {
    await new Promise(resolve => setTimeout(resolve, 0))
  })
}

describe('ResourceFormReadingPage — the reading page’s column, editable', () => {
  let viewport: ReturnType<typeof mockViewportWidth>

  beforeEach(() => {
    storage.clear()
    vi.clearAllMocks()
    viewport = mockViewportWidth(1280)
  })

  afterEach(() => {
    viewport.restore()
  })

  // The acceptance criterion itself: View → Edit keeps the same sections,
  // headings and order. The edit column stops after Attachments (activity,
  // comments and relations are not edited), so it must be a PREFIX of the view.
  it.each(KINDS)(
    'gives the %s edit column the view column’s sections and headings, in order',
    (kind, descriptor) => {
      const view = renderView(kind, descriptor)
      const reading = columnShape()
      view.unmount()

      renderEdit(kind, descriptor)
      const editing = columnShape()

      expect(editing.sections.length).toBeGreaterThan(0)
      expect(editing.sections).toEqual(
        reading.sections.slice(0, editing.sections.length)
      )
      expect(editing.headings).toEqual(
        reading.headings.slice(0, editing.headings.length)
      )
      expect(editing.headings).toEqual(
        descriptor.capabilities.attachments
          ? ['Metadata', 'Labels & metadata', 'Attachments']
          : ['Metadata', 'Labels & metadata']
      )
    }
  )

  it.each(KINDS)(
    'keeps the %s Created and Version rows and the history link while editing',
    (kind, descriptor) => {
      renderEdit(kind, descriptor)
      const panel = metadataPanel()
      expect(panel.getByText('Created')).toBeInTheDocument()
      expect(panel.getByText('Version')).toBeInTheDocument()
      expect(panel.getByText('v3')).toBeInTheDocument()
      expect(
        screen.getByTestId('metadata-version-history-link')
      ).toHaveAttribute('href', '/somewhere/versions')
    }
  )

  it.each([
    ['artifact', artifactDescriptor, 'artifact-type-select'],
    ['blueprint', blueprintDescriptor, 'blueprint-type-select'],
  ] as const)(
    'puts the %s Type, Status and Project controls in their metadata rows',
    (kind, descriptor, typeTestId) => {
      renderEdit(kind, descriptor)
      const panel = metadataPanel()
      for (const testId of [
        typeTestId,
        `${kind}-status-select`,
        `${kind}-project-select`,
      ]) {
        const control = panel.getByTestId(testId)
        // In a row, beside the row's own label — not a stacked form field.
        expect(control.closest('li')).not.toBeNull()
      }
    }
  )

  // The slug is the resource's address: locked once created, so it reads as
  // the reading page's copyable row, not a disabled input.
  it.each([
    ['artifact', artifactDescriptor, 'quarterly-report'],
    ['blueprint', blueprintDescriptor, 'code-review-rules'],
  ] as const)('shows the %s slug read-only', (kind, descriptor, slug) => {
    renderEdit(kind, descriptor)
    expect(screen.queryByTestId(`${kind}-slug-input`)).not.toBeInTheDocument()
    expect(
      metadataPanel().getByRole('button', { name: /copy slug/i })
    ).toHaveTextContent(slug)
  })

  // A prompt is addressed by slug alone, so renaming it is a legitimate edit
  // (the descriptor leaves it editable) — its row carries the input.
  it('keeps the prompt slug editable, in its row', () => {
    renderEdit('prompt', promptDescriptor)
    const input = metadataPanel().getByTestId('prompt-slug-input')
    expect(input).toHaveValue('summarise')
    expect(input).toBeEnabled()
  })

  it('shows the memory id as a read-only row', () => {
    renderEdit('memory', memoryDescriptor)
    expect(metadataPanel().getByText('ID')).toBeInTheDocument()
  })

  it('saves a Type, Status and Project changed in the column', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderEdit('artifact', artifactDescriptor)

    await user.click(screen.getByTestId('artifact-status-select'))
    await user.click(await screen.findByRole('option', { name: 'Archived' }))
    await user.click(screen.getByTestId('artifact-project-select'))
    await user.click(screen.getByTestId('artifact-type-select'))
    await user.click(await screen.findByRole('option', { name: 'Report' }))

    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await flush()

    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      status: 'archived',
      project_id: 'p2',
      type: 'report',
      // The locked slug still travels with the payload, unchanged.
      slug: 'quarterly-report',
    })
  })

  it('titles a compact select with its full value, for a truncated name', () => {
    renderEdit('artifact', artifactDescriptor)
    expect(screen.getByTestId('artifact-type-select')).toHaveAttribute(
      'title',
      'General'
    )
  })

  it('titles the compact project picker too', () => {
    renderEdit('artifact', artifactDescriptor)
    expect(screen.getByTestId('artifact-project-select')).toHaveAttribute(
      'data-title-trigger',
      'true'
    )
  })

  // A taxonomy field with no form control (the blueprint's imported subtype)
  // stays on screen as the reading page's chips.
  it('keeps a read-only taxonomy field as chips while editing', () => {
    renderEdit('blueprint', blueprintDescriptor)
    const taxonomy = within(screen.getByTestId('resource-form-taxonomy'))
    expect(taxonomy.getByText('Subtype')).toBeInTheDocument()
    expect(taxonomy.getByText('agents')).toBeInTheDocument()
    // An editable taxonomy field shows its input, not a second chip row.
    expect(
      taxonomy
        .getAllByTestId('taxonomy-group-label')
        .map(label => label.textContent)
    ).toEqual(['Subtype'])
  })

  it('renders the labels and metadata inputs in Labels & metadata', () => {
    renderEdit('artifact', artifactDescriptor)
    const taxonomy = within(screen.getByTestId('resource-form-taxonomy'))
    expect(taxonomy.getByText('Labels & metadata')).toBeInTheDocument()
    expect(taxonomy.getByTestId('artifact-labels-input')).toBeInTheDocument()
  })

  describe('extension placement (declared on the descriptor)', () => {
    it('puts the memory tags in Labels & metadata', () => {
      renderEdit('memory', memoryDescriptor, {
        extensions: { tags: <div data-testid="memory-tags-card" /> },
      })
      expect(
        within(screen.getByTestId('resource-form-taxonomy')).getByTestId(
          'memory-tags-card'
        )
      ).toBeInTheDocument()
    })

    it('puts the prompt MCP exposure in the MCP row, as its value', () => {
      renderEdit('prompt', promptDescriptor, {
        extensions: { 'mcp-exposure': <div data-testid="mcp-card" /> },
      })
      const row = screen.getByTestId('mcp-card').closest('li')
      expect(row).not.toBeNull()
      expect(row?.firstElementChild).toHaveTextContent('MCP')
      // The switch replaces the read-only value; the fact is shown once.
      expect(metadataPanel().queryByText('Exposed')).not.toBeInTheDocument()
    })

    // The real slot components, not stubs: they must not bring a heading of
    // their own, or the column stops matching the reading page's.
    it.each([
      [
        'prompt',
        promptDescriptor,
        {
          'mcp-exposure': <McpExposureSwitch value={true} onChange={vi.fn()} />,
        },
      ],
      [
        'memory',
        memoryDescriptor,
        { tags: <MemoryTagsField value={['ops']} onChange={vi.fn()} /> },
      ],
    ] as const)(
      'keeps the %s headings the reading page’s with its real extensions',
      (kind, descriptor, extensions) => {
        const view = renderView(kind, descriptor)
        const reading = columnShape()
        view.unmount()

        renderEdit(kind, descriptor, { extensions })
        const editing = columnShape()
        expect(editing.headings).toEqual(
          reading.headings.slice(0, editing.headings.length)
        )
      }
    )

    it('toggles MCP exposure from the MCP row', async () => {
      const user = userEvent.setup()
      const onChange = vi.fn()
      renderEdit('prompt', promptDescriptor, {
        extensions: {
          'mcp-exposure': (
            <McpExposureSwitch value={true} onChange={onChange} />
          ),
        },
      })
      await user.click(screen.getByTestId('prompt-mcp-expose'))
      expect(onChange).toHaveBeenCalledWith(false)
    })

    it('says "Not exposed" in the MCP row while the prompt is a draft', () => {
      renderEdit('prompt', promptDescriptor, {
        initialValues: { ...RECORDS.prompt, status: 'draft' },
        extensions: {
          'mcp-exposure': <McpExposureSwitch value={true} onChange={vi.fn()} />,
        },
      })
      expect(screen.queryByTestId('prompt-mcp-expose')).not.toBeInTheDocument()
      expect(metadataPanel().getByText('Not exposed')).toBeInTheDocument()
    })
  })

  describe('attachments', () => {
    it.each([
      ['artifact', artifactDescriptor],
      ['blueprint', blueprintDescriptor],
      ['prompt', promptDescriptor],
    ] as const)(
      'lists the %s attachments while editing',
      (kind, descriptor) => {
        renderEdit(kind, descriptor)
        const section = screen
          .getByTestId('details-column')
          .querySelector(`[data-section="${RESOURCE_SECTION_IDS.attachments}"]`)
        expect(section).not.toBeNull()
        expect(
          within(section as HTMLElement).getByTestId('attachments-panel')
        ).toHaveAttribute('data-owner', String(RECORDS[kind].id))
        // Not form state, and the page says so.
        expect(
          within(section as HTMLElement).getByText(/save immediately/i)
        ).toBeInTheDocument()
      }
    )

    it('has no Attachments section for a kind that takes none', () => {
      renderEdit('memory', memoryDescriptor)
      expect(screen.queryByTestId('attachments-panel')).not.toBeInTheDocument()
    })

    it('has no Attachments section until the resource is known', () => {
      renderEdit('artifact', artifactDescriptor, { resource: undefined })
      expect(screen.queryByTestId('attachments-panel')).not.toBeInTheDocument()
    })
  })

  // The read-only rows read the fetched resource, which a page passes when its
  // form values are mapped rather than the resource itself (memory, prompt).
  it('reads the read-only rows from `record` over the form values', () => {
    renderEdit('prompt', promptDescriptor, {
      initialValues: { name: 'Summarise', body: 'Body', status: 'draft' },
      record: { ...RECORDS.prompt, mcp_expose: false },
    })
    const panel = metadataPanel()
    expect(panel.getByText('Created')).toBeInTheDocument()
    expect(panel.getByText('Not exposed')).toBeInTheDocument()
  })

  // Folded rail + invalid submit: the invalid control is now a row in the
  // Metadata section, and it is still revealed and focused.
  it('reopens the folded column and focuses an invalid row control', async () => {
    const user = userEvent.setup()
    storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
    renderEdit('artifact', artifactDescriptor, {
      initialValues: { ...RECORDS.artifact, type: '' },
    })
    expect(screen.getByTestId('reading-details')).toHaveAttribute(
      'data-state',
      'collapsed'
    )
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await flush()
    await flush()
    expect(screen.getByTestId('reading-details')).toHaveAttribute(
      'data-state',
      'open'
    )
    const type = screen.getByTestId('artifact-type-select')
    expect(type).toHaveAttribute('aria-invalid', 'true')
    expect(type).toHaveFocus()
  })

  // Since #1181 the create pages render this same column (mode="create"):
  // nothing to show yet for Created/Version/Attachments, and every field —
  // the slug included — is an input in its row.
  describe('create mode', () => {
    function renderCreate(
      descriptor: ResourceDescriptor,
      overrides: Partial<ResourceFormReadingPageProps> = {}
    ) {
      return render(
        <MemoryRouter>
          <ShellProvider>
            <ResourceFormReadingPage
              title={`New ${descriptor.singular}`}
              descriptor={descriptor}
              mode="create"
              onSubmit={vi.fn()}
              onCancel={vi.fn()}
              {...overrides}
            />
          </ShellProvider>
        </MemoryRouter>
      )
    }

    it.each(KINDS)(
      'gives the %s create column the Metadata and Labels sections only',
      (_kind, descriptor) => {
        renderCreate(descriptor)
        expect(columnShape()).toEqual({
          sections: [RESOURCE_SECTION_IDS.metadata],
          headings: ['Metadata', 'Labels & metadata'],
        })
        expect(metadataPanel().queryByText('Created')).not.toBeInTheDocument()
      }
    )

    it.each([
      ['artifact', artifactDescriptor],
      ['blueprint', blueprintDescriptor],
    ] as const)('makes the %s slug an input in its row', (kind, descriptor) => {
      renderCreate(descriptor)
      const input = metadataPanel().getByTestId(`${kind}-slug-input`)
      expect(input.closest('li')?.firstElementChild).toHaveTextContent('Slug')
      expect(input).toBeEnabled()
    })

    it('places the real extensions as on the edit page', () => {
      const view = renderCreate(memoryDescriptor, {
        extensions: { tags: <MemoryTagsField value={[]} onChange={vi.fn()} /> },
      })
      expect(
        within(screen.getByTestId('resource-form-taxonomy')).getByTestId(
          'memory-tags-input'
        )
      ).toBeInTheDocument()
      expect(columnShape().headings).toEqual(['Metadata', 'Labels & metadata'])
      view.unmount()

      renderCreate(promptDescriptor, {
        extensions: {
          'mcp-exposure': (
            <McpExposureSwitch value={false} onChange={vi.fn()} />
          ),
        },
      })
      // A new prompt starts as a draft, so the MCP row says so.
      const row = metadataPanel().getByText('Not exposed').closest('li')
      expect(row?.firstElementChild).toHaveTextContent('MCP')
      expect(columnShape().headings).toEqual(['Metadata', 'Labels & metadata'])
    })
  })
})
