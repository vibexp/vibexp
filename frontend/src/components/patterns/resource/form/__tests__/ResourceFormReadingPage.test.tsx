import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import type { MockInstance } from 'vitest'

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
import type {
  ResourceFormHandle,
  ResourceFormReadingPageProps,
} from '../ResourceFormReadingPage'
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

  // The header is the reading page's header, made editable (#1179): the name
  // and the description are edited where they are displayed.
  describe('inline-editable header', () => {
    const HEADER_KINDS: readonly (readonly [
      string,
      ResourceDescriptor,
      string,
      string | null,
    ])[] = [
      ['artifact', artifactDescriptor, 'title', 'description'],
      ['blueprint', blueprintDescriptor, 'title', 'description'],
      ['prompt', promptDescriptor, 'name', 'description'],
      // Memory has a name but no summary: its title is inline, nothing else.
      ['memory', memoryDescriptor, 'title', null],
    ]

    function testIdOf(descriptor: ResourceDescriptor, key: string) {
      return descriptor.form?.fields.find(spec => spec.key === key)?.testId
    }

    function header() {
      const node = screen
        .getByTestId('reading-page')
        .querySelector('article header')
      expect(node).not.toBeNull()
      return node as HTMLElement
    }

    it.each(HEADER_KINDS)(
      'edits the %s name and summary in the header, not the column',
      (_kind, descriptor, nameKey, summaryKey) => {
        renderEditPage(descriptor)
        const column = screen.getByTestId('details-column')
        const title = screen.getByTestId(testIdOf(descriptor, nameKey)!)
        expect(header()).toContainElement(title)
        expect(column).not.toContainElement(title)
        expect(title).toHaveAttribute(
          'placeholder',
          `Untitled ${descriptor.singular}`
        )
        // Still a labelled control: its (visually hidden) label names it.
        const label = descriptor.fields.find(f => f.key === nameKey)?.label
        expect(within(header()).getByRole('textbox', { name: label })).toBe(
          title
        )
        if (summaryKey) {
          const summary = screen.getByTestId(testIdOf(descriptor, summaryKey)!)
          expect(header()).toContainElement(summary)
          expect(column).not.toContainElement(summary)
          expect(summary).toHaveAttribute('placeholder', 'Add a description…')
        }
      }
    )

    it('keeps an accessible heading naming what is being edited', () => {
      renderEditPage(artifactDescriptor, {
        initialValues: { title: 'Release notes' },
      })
      const heading = screen.getByRole('heading', { level: 1 })
      expect(heading).toHaveTextContent('Edit artifact: Release notes')
      expect(heading).toHaveClass('sr-only')
    })

    it('honours the field limits and the save lock', () => {
      renderEditPage(promptDescriptor, { isLoading: true })
      const name = screen.getByTestId('prompt-name-input')
      expect(name).toHaveAttribute('maxLength', '50')
      expect(name).toBeDisabled()
      expect(screen.getByTestId('prompt-description-input')).toHaveAttribute(
        'maxLength',
        '200'
      )
    })

    it('keeps a single-line title: Enter and pasted newlines add no break', async () => {
      const user = userEvent.setup()
      renderEditPage(artifactDescriptor, { initialValues: { title: 'A' } })
      const title = screen.getByTestId('artifact-title-input')
      await user.type(title, 'b{Enter}c')
      expect(title).toHaveValue('Abc')
      await user.clear(title)
      await user.click(title)
      await user.paste('one\ntwo')
      expect(title).toHaveValue('one two')
    })

    it('saves the edited title and description', async () => {
      const user = userEvent.setup()
      const { onSubmit } = renderEditPage(artifactDescriptor, {
        initialValues: {
          title: 'Old',
          description: 'Old lead',
          slug: 'a-slug',
          project_id: 'p1',
          content: 'Body',
          type: 'general',
          status: 'active',
        },
      })
      await user.clear(screen.getByTestId('artifact-title-input'))
      await user.type(screen.getByTestId('artifact-title-input'), 'New')
      await user.clear(screen.getByTestId('artifact-description-input'))
      await user.type(
        screen.getByTestId('artifact-description-input'),
        'New lead'
      )
      await user.click(screen.getByRole('button', { name: 'Save changes' }))
      await act(async () => {
        await new Promise(resolve => setTimeout(resolve, 0))
      })
      expect(onSubmit).toHaveBeenCalledTimes(1)
      expect(onSubmit.mock.calls[0][0]).toMatchObject({
        title: 'New',
        description: 'New lead',
      })
    })

    it('renders the badge row from the live form and the resource', () => {
      renderEditPage(artifactDescriptor, {
        initialValues: { title: 'A', slug: 'a-slug', status: 'draft' },
        updatedAt: '2026-09-01T10:00:00Z',
      })
      const meta = within(header()).getByTestId('resource-header-meta')
      expect(within(meta).getByText('Draft')).toBeInTheDocument()
      expect(within(meta).getByText('a-slug')).toBeInTheDocument()
      expect(meta).toHaveTextContent(/Updated/)
    })

    it('recolours the status badge as soon as Status changes', async () => {
      const user = userEvent.setup()
      Element.prototype.scrollIntoView = vi.fn()
      Element.prototype.hasPointerCapture = vi.fn(() => false)
      Element.prototype.releasePointerCapture = vi.fn()
      renderEditPage(artifactDescriptor, {
        initialValues: { title: 'A', status: 'active' },
      })
      const meta = within(header()).getByTestId('resource-header-meta')
      expect(within(meta).getByText('Active')).toBeInTheDocument()

      await user.click(screen.getByTestId('artifact-status-select'))
      await user.click(await screen.findByRole('option', { name: 'Archived' }))

      expect(within(meta).getByText('Archived')).toBeInTheDocument()
      expect(within(meta).queryByText('Active')).not.toBeInTheDocument()
    })

    // The error is already in view, so the reader's folded rail stays folded;
    // the header input is where the focus goes.
    it('reports a header-only error under the header input and focuses it', async () => {
      const user = userEvent.setup()
      Element.prototype.scrollIntoView = vi.fn()
      storage.set(STORAGE_KEYS.DETAILS_COLLAPSED, true)
      renderEditPage(artifactDescriptor, {
        initialValues: {
          title: 'A title',
          slug: 'a-slug',
          project_id: 'p1',
          content: 'Body',
          type: 'general',
          status: 'active',
        },
      })
      const title = screen.getByTestId('artifact-title-input')
      await user.clear(title)
      await user.click(screen.getByRole('button', { name: 'Save changes' }))
      await act(async () => {
        await new Promise(resolve => setTimeout(resolve, 0))
      })
      expect(title).toHaveAttribute('aria-invalid', 'true')
      expect(title).toHaveFocus()
      const describedBy = title.getAttribute('aria-describedby') ?? ''
      const message = describedBy
        .split(' ')
        .map(id => document.getElementById(id))
        .find(node => node?.textContent)
      expect(header()).toContainElement(message ?? null)
      expect(screen.getByTestId('reading-details')).toHaveAttribute(
        'data-state',
        'collapsed'
      )
    })

    it('renders kind-specific header badges beside the status', () => {
      renderEditPage(promptDescriptor, {
        initialValues: { name: 'P', status: 'published' },
        headerExtra: <span data-testid="shared-badge">Shared</span>,
      })
      expect(
        within(header()).getByTestId('resource-header-meta')
      ).toContainElement(screen.getByTestId('shared-badge'))
    })

    // Without `field-sizing: content` the one-row box would hide every wrapped
    // line, so it is sized from its text.
    describe('auto-size fallback', () => {
      let scrollHeight: MockInstance<() => number>

      beforeEach(() => {
        // Absent in jsdom, as in a browser without `field-sizing`.
        vi.stubGlobal('CSS', { supports: () => false })
        scrollHeight = vi.spyOn(
          HTMLTextAreaElement.prototype,
          'scrollHeight',
          'get'
        )
      })

      afterEach(() => {
        vi.unstubAllGlobals()
        scrollHeight.mockRestore()
      })

      it('sizes the header inputs from their content', async () => {
        const user = userEvent.setup()
        scrollHeight.mockReturnValue(64)
        renderEditPage(artifactDescriptor, {
          initialValues: { title: 'A long title', description: 'Lead' },
        })
        const title = screen.getByTestId('artifact-title-input')
        expect(title.style.height).toBe('64px')
        expect(
          screen.getByTestId('artifact-description-input').style.height
        ).toBe('64px')

        scrollHeight.mockReturnValue(96)
        await user.type(title, ' that wraps')
        expect(title.style.height).toBe('96px')
      })

      it('re-fits when the column width changes, not only the text', () => {
        const observers: (() => void)[] = []
        vi.stubGlobal(
          'ResizeObserver',
          class {
            constructor(callback: () => void) {
              observers.push(callback)
            }
            observe() {}
            disconnect() {}
          }
        )
        const clientWidth = vi
          .spyOn(HTMLTextAreaElement.prototype, 'clientWidth', 'get')
          .mockReturnValue(600)
        scrollHeight.mockReturnValue(32)
        renderEditPage(artifactDescriptor, {
          initialValues: { title: 'A long title' },
        })
        const title = screen.getByTestId('artifact-title-input')
        expect(title.style.height).toBe('32px')

        // Same text, narrower box: it wraps, so it must grow.
        clientWidth.mockReturnValue(300)
        scrollHeight.mockReturnValue(64)
        act(() => {
          observers.forEach(fire => {
            fire()
          })
        })
        expect(title.style.height).toBe('64px')
        clientWidth.mockRestore()
      })

      it('leaves sizing to CSS where the browser supports it', () => {
        vi.stubGlobal('CSS', { supports: () => true })
        scrollHeight.mockReturnValue(64)
        renderEditPage(artifactDescriptor, {
          initialValues: { title: 'A long title' },
        })
        expect(screen.getByTestId('artifact-title-input').style.height).toBe('')
      })
    })

    // A kind whose descriptor has no name text field keeps today's heading.
    it('falls back to the plain heading and lead without header fields', () => {
      const bare: ResourceDescriptor = {
        ...artifactDescriptor,
        form: {
          fields: artifactDescriptor.form.fields.filter(
            spec => spec.key !== 'title' && spec.key !== 'description'
          ),
        },
      }
      renderEditPage(bare, { description: 'A lead line' })
      expect(
        screen.getByRole('heading', { level: 1, name: 'Edit artifact' })
      ).not.toHaveClass('sr-only')
      expect(screen.getByText('A lead line')).toBeInTheDocument()
      expect(
        screen.queryByTestId('resource-header-meta')
      ).not.toBeInTheDocument()
    })
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

  // #1181: the four create pages render here too, so creating a resource looks
  // like editing it.
  describe('create mode', () => {
    function renderCreatePage(
      descriptor: ResourceDescriptor,
      overrides: Partial<ResourceFormReadingPageProps> = {}
    ) {
      return renderEditPage(descriptor, {
        title: `Create ${descriptor.singular}`,
        mode: 'create',
        ...overrides,
      })
    }

    function headerMeta() {
      return within(
        screen.getByTestId('reading-page').querySelector('article header')!
      ).getByTestId('resource-header-meta')
    }

    it.each(EDIT_KINDS)(
      'renders the %s create form inside the reading shell',
      (_kind, descriptor) => {
        renderCreatePage(descriptor)
        expect(screen.getByTestId('reading-page')).toHaveAttribute(
          'data-presentation',
          'editing'
        )
        const column = within(screen.getByTestId('details-column'))
        expect(
          column.getByRole('button', {
            name: `Create ${descriptor.singular}`,
          })
        ).toHaveClass('bg-primary')
        expect(column.getByRole('button', { name: 'Cancel' })).toBeEnabled()
      }
    )

    it('opens on an empty, placeholder-titled header with no badge row', () => {
      renderCreatePage(blueprintDescriptor, { initialValues: {} })
      expect(screen.getByTestId('blueprint-title-input')).toHaveAttribute(
        'placeholder',
        'Untitled blueprint'
      )
      expect(screen.getByPlaceholderText('Add a description…')).toHaveValue('')
      // Nothing is saved yet: no Updated line, and no slug chip until the
      // title gives the slug a value. The status badge is the form's default.
      expect(headerMeta()).not.toHaveTextContent(/Updated/)
      expect(headerMeta().querySelector('code, button')).not.toBeInTheDocument()
    })

    it('shows the auto-filled slug read-only in the header while the field stays editable', async () => {
      const user = userEvent.setup()
      renderCreatePage(artifactDescriptor)
      await user.type(screen.getByTestId('artifact-title-input'), 'My Draft')
      const chip = within(headerMeta()).getByText('my-draft')
      // A plain chip, not the click-to-copy button edit renders: there is
      // nothing to copy an address of until the resource exists.
      expect(chip.tagName).toBe('CODE')
      expect(within(headerMeta()).queryByRole('button')).toBeNull()
      expect(screen.getByTestId('artifact-slug-input')).toBeEnabled()
      expect(screen.getByTestId('artifact-slug-input')).toHaveValue('my-draft')
    })

    it('surfaces a blank required title under the header input and focuses it', async () => {
      const user = userEvent.setup()
      Element.prototype.scrollIntoView = vi.fn()
      renderCreatePage(artifactDescriptor, {
        initialValues: { content: 'Body', project_id: 'p1', type: 'general' },
      })
      const title = screen.getByTestId('artifact-title-input')
      await user.click(screen.getByRole('button', { name: 'Create artifact' }))
      await act(async () => {
        await new Promise(resolve => setTimeout(resolve, 0))
      })
      expect(title).toHaveAttribute('aria-invalid', 'true')
      expect(title).toHaveFocus()
      expect(
        within(
          screen.getByTestId('reading-page').querySelector('article header')!
        ).getByText('Title is required')
      ).toBeInTheDocument()
    })

    it('says Creating… while the create runs', () => {
      renderCreatePage(artifactDescriptor, { isLoading: true })
      const column = within(screen.getByTestId('details-column'))
      expect(column.getByRole('button', { name: 'Creating…' })).toBeDisabled()
    })

    it('disables only Create when the page says the submit cannot succeed', () => {
      renderCreatePage(memoryDescriptor, { saveDisabled: true })
      const column = within(screen.getByTestId('details-column'))
      expect(
        column.getByRole('button', { name: 'Create memory' })
      ).toBeDisabled()
      expect(column.getByRole('button', { name: 'Cancel' })).toBeEnabled()
      expect(screen.getByTestId('memory-content-textarea')).toBeEnabled()
    })

    it('hands the page a handle that reads and submits the form', async () => {
      const ref = createRef<ResourceFormHandle>()
      const { onSubmit } = renderCreatePage(memoryDescriptor, {
        ref,
        initialValues: { text: 'Remember this', project_id: 'p1' },
      })
      expect(ref.current?.getValues()).toEqual(
        expect.objectContaining({ text: 'Remember this' })
      )
      await act(async () => {
        ref.current?.submit()
        await new Promise(resolve => setTimeout(resolve, 0))
      })
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ text: 'Remember this', project_id: 'p1' })
      )
    })
  })
})
