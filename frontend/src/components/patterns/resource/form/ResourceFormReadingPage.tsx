import { Info, Save, Tags, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useRef, useState } from 'react'

import { useShell } from '@/components/layout/ShellContext'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { ResourceHeaderMeta } from '@/components/resource-detail/ResourceHeaderMeta'
import { Form } from '@/components/ui/form'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'

import { fieldOfRole } from '../fieldOfRole'
import { fieldLabel, fieldTone, statusFieldOf } from '../statusTone'
import type { ResourceDescriptor } from '../types'
import type { ResourceFormValues } from './buildFormSchema'
import { formSaveLabel } from './formLabels'
import type { ResourceFormPageProps } from './ResourceFormPage'
import { useResourceForm } from './useResourceForm'

/** Section ids, exported for the tests that assert the `data-section` anchors. */
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
  /**
   * The page's name — "Edit artifact". The visible `<h1>` only for a kind
   * whose name is not edited in the header; otherwise the accessible heading.
   */
  title: string
  /**
   * Lead line under the title, for a kind whose header is not inline-editable.
   * Replaced by the badge row and the description input when it is.
   */
  description?: ReactNode
  /** ISO timestamp of the last edit — "Updated <relative>" in the header. */
  updatedAt?: string
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

/** A form value as display text; anything that is not a string reads as absent. */
function valueOf(values: ResourceFormValues | undefined, key: string) {
  const value = new Map(Object.entries(values ?? {})).get(key)
  return typeof value === 'string' ? value : ''
}

/**
 * The header's accessible `<h1>` once the visible name is an input: the page's
 * name plus the resource's name as it was loaded, so a screen reader still
 * lands on a heading that says what is being edited. The live input is the
 * editable copy and would re-announce on every keystroke.
 */
function headingText(
  title: string,
  descriptor: ResourceDescriptor,
  initialValues: ResourceFormValues | undefined
) {
  const nameKey = fieldOfRole(descriptor, 'name')?.key
  const name = nameKey ? valueOf(initialValues, nameKey).trim() : ''
  return name ? `${title}: ${name}` : title
}

/**
 * The generated form (#913) rendered in the reading shell (#916).
 *
 * View and edit are the same document, so they get the same layout: the body
 * editor takes the article slot at the identical reading measure and gutters, the
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
  updatedAt,
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
  // Read by the invalid-submit handler, which is handed to the hook that
  // produces the keys.
  const headerKeysNow = useRef<readonly string[]>([])

  const {
    form,
    formElRef,
    onFormSubmit,
    submit,
    isDirty,
    bodyNode,
    detailsNode,
    taxonomyNode,
    titleNode,
    summaryNode,
    headerKeys,
  } = useResourceForm({
    descriptor,
    mode,
    initialValues,
    onSubmit,
    isLoading,
    renderBody,
    metadataRequiredKeys,
    metadataReservedKeys,
    inlineHeader: true,
    // Most fields live in the details column, and the column folds to a 48px
    // rail that renders none of them. A validation error the reader cannot
    // reach is one they cannot fix, so a failed submit reopens the column —
    // whichever surface is live, since below `lg` the details are a sheet with
    // an entirely separate open state. Errors only in the header are already
    // in view, so they leave the column as the reader had it.
    onInvalidSubmit: invalidKeys => {
      const headerOnly =
        invalidKeys.length > 0 &&
        invalidKeys.every(key => headerKeysNow.current.includes(key))
      if (!headerOnly) {
        if (isDesktop) {
          if (!detailsOpen) forcedColumnOpen.current = true
          setDetailsOpen(true)
        } else {
          setDetailsSheetOpen(true)
        }
      }
      setRevealErrors(n => n + 1)
    },
  })

  useEffect(() => {
    headerKeysNow.current = headerKeys
  })

  // The header's badge row reads the LIVE form, so changing Status in the
  // column recolours the badge at once — the header is the same header as on
  // the reading page, not a snapshot of the saved resource.
  const statusField = statusFieldOf(descriptor)
  const slugKey = descriptor.form?.fields.find(
    spec => spec.pattern === 'slug'
  )?.key
  const statusValue = form.watch(statusField?.key ?? '__no_status_field__')
  const slugValue = form.watch(slugKey ?? '__no_slug_field__')
  const inlineHeader = titleNode !== null || summaryNode !== null

  const headerDescription = inlineHeader ? (
    <>
      <ResourceHeaderMeta
        status={
          typeof statusValue === 'string' && statusValue
            ? {
                value: fieldLabel(statusField, statusValue),
                tone: fieldTone(statusField, statusValue),
              }
            : undefined
        }
        address={
          typeof slugValue === 'string' && slugValue
            ? { value: slugValue, copyable: mode === 'edit' }
            : undefined
        }
        updatedAt={updatedAt}
      />
      {summaryNode && <div className="mt-2">{summaryNode}</div>}
    </>
  ) : (
    description
  )

  const heading = titleNode ? (
    <>
      <h1 className="sr-only">
        {headingText(title, descriptor, initialValues)}
      </h1>
      {titleNode}
    </>
  ) : undefined

  // Give the reader their folded rail back on the way out. `setDetailsOpen`
  // writes `DETAILS_COLLAPSED`, a persisted app-wide preference, so without
  // this one mistyped field would silently un-fold every reading page from now
  // on — the preference #916 exists to stop edit mode throwing away. Skipped
  // when the column is already folded at exit; a reader who refolds it and then
  // reopens it deliberately is indistinguishable from one who left it as we
  // forced it, and erring towards their stored preference is the safer half.
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
        heading={heading}
        description={headerDescription}
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
