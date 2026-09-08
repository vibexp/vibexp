import { Info, Save, Tags, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useRef, useState } from 'react'

import { useShell } from '@/components/layout/ShellContext'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { Form } from '@/components/ui/form'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'

import { formSaveLabel } from './formLabels'
import type { ResourceFormPageProps } from './ResourceFormPage'
import { useResourceForm } from './useResourceForm'

/** Section ids, exported so tests and rail deep-links can address them. */
export const RESOURCE_FORM_SECTION_IDS = {
  details: 'form-details',
  taxonomy: 'form-taxonomy',
} as const

/**
 * How long to wait after reopening the details surface before scrolling to the
 * first invalid control. The column's fields are unmounted while it is folded,
 * so there is nothing to scroll to until React has committed the reopen.
 */
const REVEAL_ERROR_DELAY_MS = 0

export interface ResourceFormReadingPageProps extends ResourceFormPageProps {
  /** The `<h1>` — "Edit artifact". */
  title: string
  /** Lead line under the title; the resource's own name, usually. */
  description?: ReactNode
  /** Where Cancel goes. Called only once the unsaved-changes guard clears. */
  onCancel: () => void
  /** Forwarded to the Save action, for pages an e2e spec addresses by id. */
  saveTestId?: string
  /**
   * Unsaved state the form does not own — a page's `extensions` are its own
   * `useState` and are invisible to react-hook-form, so the memory's tags and
   * the prompt's MCP toggle would otherwise be discarded by Cancel or by a tab
   * close without any prompt.
   */
  extraDirty?: boolean
}

/**
 * The generated form (#913) rendered in the reading shell (#916).
 *
 * View and edit are the same document, so they get the same layout: the body
 * editor takes the article slot at the identical 72ch measure and gutters, the
 * form's Details and Taxonomy controls become `ReadingSection`s in the details
 * column — folding to the icon rail from the same header toggle as on the
 * detail page — and Save/Cancel are `ReadingAction`s, so they render as the
 * column's button grid, as rail icons when it is folded, and as chips under the
 * title on phones.
 *
 * The form element lives in the article while the details controls render in
 * the `<aside>` beside it. That is fine and deliberate: `Form` is react-hook-
 * form's `FormProvider`, which renders no DOM of its own, and every control is
 * registered through that context rather than through DOM containment. Save
 * therefore submits the whole form no matter which slot a field sits in.
 */
export function ResourceFormReadingPage({
  title,
  description,
  onCancel,
  saveTestId,
  extraDirty = false,
  descriptor,
  mode,
  initialValues,
  onSubmit,
  isLoading = false,
  extensions,
  renderBody,
  metadataRequiredKeys,
  metadataReservedKeys,
}: Readonly<ResourceFormReadingPageProps>) {
  const { isDesktop, detailsOpen, setDetailsOpen, setDetailsSheetOpen } =
    useShell()

  // Refs so the unmount cleanup below reads the latest values without
  // re-subscribing on every shell change. Written from an effect, never during
  // render — the React Compiler lint rejects the latter, and a ref written mid
  // render is not guaranteed to survive a discarded one.
  const forcedColumnOpen = useRef(false)
  const detailsOpenNow = useRef(detailsOpen)
  const setDetailsOpenNow = useRef(setDetailsOpen)
  useEffect(() => {
    detailsOpenNow.current = detailsOpen
    setDetailsOpenNow.current = setDetailsOpen
  })

  const [revealErrors, setRevealErrors] = useState(0)

  const {
    form,
    formElRef,
    onFormSubmit,
    submit,
    isDirty,
    bodyNode,
    detailsNode,
    taxonomyNode,
  } = useResourceForm({
    descriptor,
    mode,
    initialValues,
    onSubmit,
    isLoading,
    renderBody,
    metadataRequiredKeys,
    metadataReservedKeys,
    // Most fields live in the details column, and the column folds to a 48px
    // rail that renders none of them. A validation error the reader cannot
    // reach is one they cannot fix, so a failed submit reopens the column —
    // whichever surface is live, since below `lg` the details are a sheet with
    // an entirely separate open state.
    onInvalidSubmit: () => {
      if (isDesktop) {
        if (!detailsOpen) forcedColumnOpen.current = true
        setDetailsOpen(true)
      } else {
        setDetailsSheetOpen(true)
      }
      setRevealErrors(n => n + 1)
    },
  })

  // Give the reader their folded rail back on the way out. `setDetailsOpen`
  // writes `DETAILS_COLLAPSED`, a persisted app-wide preference, so without
  // this one mistyped field would silently un-fold every reading page from now
  // on — the preference #916 exists to stop edit mode throwing away. Skipped
  // when they reopened or refolded it themselves in the meantime.
  useEffect(
    () => () => {
      if (forcedColumnOpen.current && detailsOpenNow.current) {
        setDetailsOpenNow.current(false)
      }
    },
    []
  )

  // …and scroll to the first error once the surface holding it has mounted.
  // The fields do not exist while the column is folded, so react-hook-form's
  // own `shouldFocusError` has no ref to focus at validation time.
  useEffect(() => {
    if (revealErrors === 0) return
    const timer = setTimeout(() => {
      const invalid = document.querySelector<HTMLElement>(
        '[aria-invalid="true"]'
      )
      if (typeof invalid?.scrollIntoView === 'function') {
        invalid.scrollIntoView({ block: 'center', behavior: 'smooth' })
      }
      invalid?.focus()
    }, REVEAL_ERROR_DELAY_MS)
    return () => {
      clearTimeout(timer)
    }
  }, [revealErrors])

  const { confirmLeave } = useUnsavedChanges(
    (isDirty || extraDirty) && !isLoading
  )

  const extensionNodes = new Map(Object.entries(extensions ?? {}))
  const declaredExtensions = descriptor.form?.extensions ?? []

  const actions: ReadingAction[] = [
    {
      id: 'save',
      label: isLoading ? 'Saving…' : formSaveLabel(descriptor, mode),
      icon: Save,
      emphasis: 'primary',
      disabled: isLoading,
      onClick: submit,
      testId: saveTestId,
    },
    {
      id: 'cancel',
      label: 'Cancel',
      icon: X,
      disabled: isLoading,
      onClick: () => {
        if (confirmLeave()) onCancel()
      },
    },
  ]

  // The extension slots (the prompt's MCP exposure card, the memory's tags)
  // keep the position the standalone layout gives them — under the details
  // fields — rather than becoming rail entries of their own: they are
  // self-titled cards, and the descriptor declares only their names.
  const extensionCards = declaredExtensions
    .map(name => {
      const node = extensionNodes.get(name)
      return node ? <div key={name}>{node}</div> : null
    })
    .filter(Boolean)

  const sections: ReadingSection[] = [
    {
      id: RESOURCE_FORM_SECTION_IDS.details,
      label: 'Details',
      icon: Info,
      content: (detailsNode !== null || extensionCards.length > 0) && (
        <div className="space-y-4" data-testid="resource-form-details">
          {detailsNode}
          {extensionCards}
        </div>
      ),
    },
    {
      id: RESOURCE_FORM_SECTION_IDS.taxonomy,
      label: 'Labels & metadata',
      icon: Tags,
      content: taxonomyNode && (
        <div className="space-y-4" data-testid="resource-form-taxonomy">
          {taxonomyNode}
        </div>
      ),
    },
  ]

  return (
    <Form {...form}>
      <ReadingPage
        presentation="editing"
        title={title}
        description={description}
        actions={actions}
        sections={sections}
      >
        <form
          ref={formElRef}
          data-testid="resource-form"
          className="space-y-4"
          onSubmit={onFormSubmit}
        >
          {bodyNode}
        </form>
      </ReadingPage>
    </Form>
  )
}
