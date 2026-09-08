import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ShellProvider } from '@/components/layout/ShellContext'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { mockViewportWidth } from '@/lib/testing/matchMedia'
import { storage } from '@/utils/storage'

// The picker is an async combobox over the projects API; stub it to a button
// that selects a fixed project, as the four page suites already do.
vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    onChange,
    id,
    'data-testid': testId,
  }: {
    onChange: (id: string | null) => void
    id?: string
    'data-testid'?: string
  }) => (
    <button
      type="button"
      id={id}
      data-testid={testId}
      onClick={() => {
        onChange('p1')
      }}
    >
      select project
    </button>
  ),
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ id: 't1', slug: 'general', name: 'General' }],
    isLoading: false,
  }),
}))

import { artifactDescriptor } from '../../descriptors/artifact'
import { blueprintDescriptor } from '../../descriptors/blueprint'
import { memoryDescriptor } from '../../descriptors/memory'
import { promptDescriptor } from '../../descriptors/prompt'
import type { ResourceDescriptor } from '../../types'
import type { ResourceFormReadingPageProps } from '../ResourceFormReadingPage'
import {
  RESOURCE_FORM_SECTION_IDS,
  ResourceFormReadingPage,
} from '../ResourceFormReadingPage'

const EDIT_KINDS: readonly (readonly [string, ResourceDescriptor])[] = [
  ['prompt', promptDescriptor],
  ['artifact', artifactDescriptor],
  ['blueprint', blueprintDescriptor],
  ['memory', memoryDescriptor],
]

function renderEditPage(
  descriptor: ResourceDescriptor,
  overrides: Partial<ResourceFormReadingPageProps> = {}
) {
  const onSubmit = vi.fn().mockResolvedValue(undefined)
  const onCancel = vi.fn()
  const view = render(
    <ShellProvider>
      <ResourceFormReadingPage
        title={`Edit ${descriptor.singular}`}
        descriptor={descriptor}
        mode="edit"
        onSubmit={onSubmit}
        onCancel={onCancel}
        {...overrides}
      />
    </ShellProvider>
  )
  return { onSubmit, onCancel, view }
}

describe('ResourceFormReadingPage', () => {
  let viewport: ReturnType<typeof mockViewportWidth>

  beforeEach(() => {
    storage.clear()
    vi.clearAllMocks()
    viewport = mockViewportWidth(1280)
  })

  afterEach(() => {
    viewport.restore()
  })

  it.each(EDIT_KINDS)(
    'renders the %s edit form inside the reading shell',
    (_kind, descriptor) => {
      renderEditPage(descriptor)
      const page = screen.getByTestId('reading-page')
      expect(page).toHaveAttribute('data-presentation', 'editing')
      // Every control the descriptor declares still resolves by its testid,
      // whichever slot it now sits in.
      for (const spec of descriptor.form?.fields ?? []) {
        if (spec.testId) {
          expect(screen.getByTestId(spec.testId)).toBeInTheDocument()
        }
      }
    }
  )

  // The body is the article; everything else is the details column. That split
  // is the whole point of the issue, so it is asserted by containment rather
  // than by "the field exists somewhere on the page".
  it.each(EDIT_KINDS)(
    'puts the %s body in the article and the rest in the details column',
    (_kind, descriptor) => {
      renderEditPage(descriptor)
      const article = screen
        .getByTestId('reading-page')
        .querySelector('article')
      const column = screen.getByTestId('details-column')

      const bodySpec = descriptor.form?.fields.find(
        spec => spec.section === 'body' && spec.testId
      )
      expect(bodySpec).toBeDefined()
      expect(article).toContainElement(screen.getByTestId(bodySpec!.testId!))
      expect(within(article!).getByTestId('resource-form')).toBeInTheDocument()

      expect(
        within(column).getByTestId('resource-form-details')
      ).toBeInTheDocument()
      expect(
        within(column).getByTestId('resource-form-taxonomy')
      ).toBeInTheDocument()

      // Each one is a real rail-addressable section, not just a div in the
      // column — that is what lets the folded rail scroll to it.
      for (const id of Object.values(RESOURCE_FORM_SECTION_IDS)) {
        expect(column.querySelector(`[data-section="${id}"]`)).not.toBeNull()
      }
    }
  )

  it('renders Save as the primary action and Cancel beside it', () => {
    renderEditPage(artifactDescriptor)
    const column = within(screen.getByTestId('details-column'))
    const save = column.getByRole('button', { name: 'Save changes' })
    expect(save).toHaveClass('bg-primary')
    expect(column.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  })

  it('forwards a save testid so a page can address its own button', () => {
    renderEditPage(promptDescriptor, { saveTestId: 'prompt-save-button' })
    expect(screen.getByTestId('prompt-save-button')).toBeInTheDocument()
  })

  it('shows the saving label and disables both actions while saving', () => {
    renderEditPage(artifactDescriptor, { isLoading: true })
    const column = within(screen.getByTestId('details-column'))
    expect(column.getByRole('button', { name: 'Saving…' })).toBeDisabled()
    expect(column.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('renders the descriptor extension slots in the details column', () => {
    renderEditPage(memoryDescriptor, {
      extensions: { tags: <div data-testid="memory-tags-card" /> },
    })
    expect(
      within(screen.getByTestId('details-column')).getByTestId(
        'memory-tags-card'
      )
    ).toBeInTheDocument()
  })

  it('submits the whole form from the actions rail, fields in both slots', async () => {
    const user = userEvent.setup()
    const { onSubmit } = renderEditPage(artifactDescriptor, {
      initialValues: {
        title: 'A title',
        slug: 'a-slug',
        project_id: 'p1',
        content: 'Body text',
        type: 'general',
        status: 'active',
      },
    })
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(onSubmit).toHaveBeenCalledTimes(1)
    // The details column's fields reach the payload even though they render
    // outside the <form> element — the FormProvider is what binds them.
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      title: 'A title',
      content: 'Body text',
    })
  })

  describe('unsaved changes', () => {
    it('leaves without asking when nothing has been edited', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const { onCancel } = renderEditPage(artifactDescriptor, {
        initialValues: { title: 'A title', content: 'Body' },
      })
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(confirmSpy).not.toHaveBeenCalled()
      expect(onCancel).toHaveBeenCalledTimes(1)
    })

    it('asks before discarding edits, and stays put when declined', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      const { onCancel } = renderEditPage(artifactDescriptor, {
        initialValues: { title: 'A title', content: 'Body' },
      })
      await user.type(screen.getByTestId('artifact-title-input'), '!')
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(confirmSpy).toHaveBeenCalled()
      expect(onCancel).not.toHaveBeenCalled()
    })

    it('leaves once the reader confirms', async () => {
      const user = userEvent.setup()
      vi.spyOn(window, 'confirm').mockReturnValue(true)
      const { onCancel } = renderEditPage(artifactDescriptor, {
        initialValues: { title: 'A title', content: 'Body' },
      })
      await user.type(screen.getByTestId('artifact-title-input'), '!')
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(onCancel).toHaveBeenCalledTimes(1)
    })
  })

  // Most fields live in the column, and a folded rail renders none of them —
  // so a validation error there would otherwise be unreachable.
  it('reopens a folded details column when a submit fails validation', async () => {
    const user = userEvent.setup()
    storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
    renderEditPage(artifactDescriptor, {
      initialValues: { title: '', content: 'Body' },
    })
    expect(screen.getByTestId('reading-details')).toHaveAttribute(
      'data-state',
      'collapsed'
    )
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(screen.getByTestId('reading-details')).toHaveAttribute(
      'data-state',
      'open'
    )
  })

  // Below `lg` the details are a Sheet with an entirely separate open state,
  // so opening the column does nothing there.
  it('opens the details sheet instead when a submit fails below lg', async () => {
    const user = userEvent.setup()
    viewport.setWidth(900)
    renderEditPage(artifactDescriptor, {
      initialValues: { title: '', content: 'Body' },
    })
    expect(
      screen.queryByTestId('reading-details-sheet')
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(screen.getByTestId('reading-details-sheet')).toBeInTheDocument()
  })

  // `DETAILS_COLLAPSED` is persisted and app-wide: a mistyped field must not
  // silently un-fold every reading page from then on.
  it('restores the reader’s folded rail on the way out', async () => {
    const user = userEvent.setup()
    storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
    const { view } = renderEditPage(artifactDescriptor, {
      initialValues: { title: '', content: 'Body' },
    })
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(storage.getJSON(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe(false)
    view.unmount()
    expect(storage.getJSON(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe(true)
  })

  it('leaves an already-open column alone on the way out', async () => {
    const user = userEvent.setup()
    const { view } = renderEditPage(artifactDescriptor, {
      initialValues: { title: '', content: 'Body' },
    })
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    view.unmount()
    expect(storage.getJSON(STORAGE_KEYS.DETAILS_COLLAPSED)).toBe(false)
  })

  // Opening the panel is only half of it: the reader still has to find the
  // field that failed.
  it('scrolls the first invalid control into view', async () => {
    const user = userEvent.setup()
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView
    renderEditPage(artifactDescriptor, {
      initialValues: { title: '', content: 'Body' },
    })
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await act(async () => {
      await new Promise(resolve => setTimeout(resolve, 0))
    })
    expect(scrollIntoView).toHaveBeenCalled()
    expect(screen.getByTestId('artifact-title-input')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  })

  // A page's `extensions` are its own useState and invisible to
  // react-hook-form, so the guard has to be told about them.
  describe('extraDirty', () => {
    it('asks before Cancel discards extension-only edits', async () => {
      const user = userEvent.setup()
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
      const { onCancel } = renderEditPage(memoryDescriptor, {
        initialValues: { text: 'Body' },
        extraDirty: true,
      })
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(confirmSpy).toHaveBeenCalled()
      expect(onCancel).not.toHaveBeenCalled()
    })

    it('registers the tab-close guard for extension-only edits', () => {
      const addSpy = vi.spyOn(window, 'addEventListener')
      renderEditPage(memoryDescriptor, {
        initialValues: { text: 'Body' },
        extraDirty: true,
      })
      expect(addSpy.mock.calls.some(call => call[0] === 'beforeunload')).toBe(
        true
      )
    })
  })
})
