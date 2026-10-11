/**
 * The generated form's Status control over the team's status catalog (#1263):
 * the descriptor's system values, then the team's custom statuses — and what
 * the form does with a status the catalog does not (or no longer) contain.
 */
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'

import { ShellProvider } from '@/components/layout/ShellContext'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: () => <button type="button">select project</button>,
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ id: 't1', slug: 'general', name: 'General' }],
    isLoading: false,
  }),
}))

const catalog = vi.hoisted(() => ({
  statuses: [] as unknown[],
  isLoading: false,
}))
vi.mock('@/hooks/useStatusCatalog', () => ({
  useStatusCatalog: () => catalog,
}))

import { artifactDescriptor } from '../../descriptors/artifact'
import { blueprintDescriptor } from '../../descriptors/blueprint'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import type { ResourceDescriptor } from '../../types'
import type { ResourceFormHandle } from '../ResourceFormReadingPage'
import { ResourceFormReadingPage } from '../ResourceFormReadingPage'

const IN_REVIEW = { slug: 'in-review', name: 'In review', is_system: false }

const FORM_KINDS: readonly (readonly [string, ResourceDescriptor])[] = [
  ['prompt', promptDescriptor],
  ['artifact', artifactDescriptor],
  ['blueprint', blueprintDescriptor],
  ['memory', memoryDescriptor],
]

/** A resource every kind's schema accepts, carrying the given status. */
function resource(status: string) {
  return {
    name: 'A name',
    title: 'A title',
    slug: 'a-slug',
    project_id: 'p1',
    type: 'general',
    body: 'Body',
    content: 'Content',
    text: 'Text',
    status,
  }
}

function renderEdit(descriptor: ResourceDescriptor, status: string) {
  const onSubmit = vi.fn().mockResolvedValue(undefined)
  const ref = createRef<ResourceFormHandle>()
  render(
    <ShellProvider>
      <ResourceFormReadingPage
        ref={ref}
        descriptor={descriptor}
        title={`Edit ${descriptor.singular}`}
        mode="edit"
        initialValues={resource(status)}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />
    </ShellProvider>
  )
  return { onSubmit, ref }
}

function statusSelect(descriptor: ResourceDescriptor) {
  return screen.getByTestId(`${descriptor.kind}-status-select`)
}

function headerBadge(text: string) {
  return within(screen.getByTestId('resource-header-meta')).getByText(text)
}

async function submit(ref: React.RefObject<ResourceFormHandle | null>) {
  await act(async () => {
    ref.current?.submit()
    await new Promise(resolve => setTimeout(resolve, 0))
  })
}

let viewport: ReturnType<typeof mockViewportWidth>

beforeAll(() => {
  // Radix Select needs layout APIs jsdom lacks.
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn(() => false)
  Element.prototype.releasePointerCapture = vi.fn()
})

beforeEach(() => {
  storage.clear()
  viewport = mockViewportWidth(1280)
  catalog.statuses = []
  catalog.isLoading = false
})

afterEach(() => {
  viewport.restore()
})

describe('generated form — status catalog', () => {
  it.each(FORM_KINDS)(
    'offers the %s team statuses after the system ones',
    async (_kind, descriptor) => {
      catalog.statuses = [IN_REVIEW]
      const user = userEvent.setup()
      const system =
        descriptor.fields.find(field => field.role === 'status')
          ?.statusValues ?? []
      renderEdit(descriptor, system[0])

      await user.click(statusSelect(descriptor))
      const options = (await screen.findAllByRole('option')).map(
        option => option.textContent
      )

      expect(options).toHaveLength(system.length + 1)
      expect(options.at(-1)).toBe('In review')
    }
  )

  it.each(FORM_KINDS)(
    'saves a %s with a custom status, sending its slug unchanged',
    async (_kind, descriptor) => {
      catalog.statuses = [IN_REVIEW]
      const user = userEvent.setup()
      const system =
        descriptor.fields.find(field => field.role === 'status')
          ?.statusValues ?? []
      const { ref, onSubmit } = renderEdit(descriptor, system[0])

      await user.click(statusSelect(descriptor))
      await user.click(await screen.findByRole('option', { name: 'In review' }))
      await submit(ref)

      expect(onSubmit).toHaveBeenCalledTimes(1)
      expect(onSubmit.mock.calls[0][0]).toMatchObject({ status: 'in-review' })
    }
  )

  it('shows the display name of the custom status a resource carries, in a neutral badge', () => {
    catalog.statuses = [IN_REVIEW]
    renderEdit(artifactDescriptor, 'in-review')

    expect(statusSelect(artifactDescriptor)).toHaveTextContent('In review')
    expect(headerBadge('In review').className).toContain('bg-muted')
  })

  it('keeps a system status in its own tone and wording', () => {
    catalog.statuses = [IN_REVIEW]
    renderEdit(artifactDescriptor, 'active')

    expect(headerBadge('Active').className).toContain('bg-success')
  })

  // Deleted elsewhere: the form must open on what the resource really holds,
  // not on a blank trigger, and saving must not throw or swap the value.
  it.each(FORM_KINDS)(
    'opens and saves a %s whose status is no longer in the catalog',
    async (_kind, descriptor) => {
      catalog.statuses = [IN_REVIEW]
      const { ref, onSubmit } = renderEdit(descriptor, 'deleted-elsewhere')

      expect(statusSelect(descriptor)).toHaveTextContent('deleted-elsewhere')
      expect(headerBadge('deleted-elsewhere').className).toContain('bg-muted')

      await submit(ref)

      expect(onSubmit).toHaveBeenCalledTimes(1)
      expect(onSubmit.mock.calls[0][0]).toMatchObject({
        status: 'deleted-elsewhere',
      })
    }
  )

  it('falls back to the raw slug while the catalog is still loading', () => {
    catalog.isLoading = true
    renderEdit(memoryDescriptor, 'in-review')

    expect(statusSelect(memoryDescriptor)).toHaveTextContent('in-review')
    expect(headerBadge('in-review').className).toContain('bg-muted')
  })

  it('does not offer the missing value twice once the catalog contains it', async () => {
    catalog.statuses = [IN_REVIEW]
    const user = userEvent.setup()
    renderEdit(memoryDescriptor, 'in-review')

    await user.click(statusSelect(memoryDescriptor))
    const options = (await screen.findAllByRole('option')).map(
      option => option.textContent
    )

    expect(options.filter(option => option === 'In review')).toHaveLength(1)
    expect(options).not.toContain('in-review')
  })
})
