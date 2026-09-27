import { Save, X } from 'lucide-react'
import type { ReactNode, Ref } from 'react'
import { useEffect, useImperativeHandle, useRef, useState } from 'react'

import { ResourceAttachments } from '@/components/attachments/ResourceAttachments'
import { useShell } from '@/components/layout/ShellContext'
import type { VersionHistoryMeta } from '@/components/metadata/MetadataPanel'
import {
  type ReadingAction,
  ReadingPage,
  type ReadingSection,
} from '@/components/patterns/reading-page'
import { ResourceHeaderMeta } from '@/components/resource-detail/ResourceHeaderMeta'
import type { ResourceRef } from '@/components/resource-detail/ResourceReadingPage'
import {
  RESOURCE_SECTION_CHROME,
  RESOURCE_SECTION_IDS,
} from '@/components/resource-detail/resourceSections'
import { Form } from '@/components/ui/form'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'

import { fieldOfRole } from '../fieldOfRole'
import { ResourceMetadataSection } from '../ResourceMetadataSection'
import {
  ReadOnlyTaxonomyGroups,
  TaxonomyPanel,
} from '../ResourceTaxonomySection'
import { fieldLabel, fieldTone, statusFieldOf } from '../statusTone'
import type { ResourceDescriptor } from '../types'
import type { ResourceFormValues } from './buildFormSchema'
import type { ResourceFormMode } from './formLabels'
import { formSaveLabel } from './formLabels'
import type { BodySlotProps } from './ResourceFormControl'
import { useResourceForm } from './useResourceForm'

/**
 * How long to wait after reopening the details surface before scrolling to the
 * first invalid control. The column's fields are unmounted while it is folded,
 * so there is nothing to scroll to until React has committed the reopen.
 */
const REVEAL_ERROR_DELAY_MS = 0

/**
 * What a page can reach of the form it hands its fields to — for a Save that
 * lives outside the page, and for an extension that has to read the form.
 */
export interface ResourceFormHandle {
  submit: () => void
  /**
   * The form's current values.
   *
   * The page owns the extension slots but not the form, and an extension
   * occasionally has to read it: the prompt editor's template loader must not
   * replace a body the user has already typed into, and re-seeding through
   * `initialValues` would otherwise discard every OTHER field along with it.
   * Reading, never writing — a slot that wants to change a field re-seeds.
   */
  getValues: () => ResourceFormValues
}

export interface ResourceFormReadingPageProps {
  descriptor: ResourceDescriptor
  mode: ResourceFormMode
  /** The resource being edited, as a value map. Omit when creating. */
  initialValues?: ResourceFormValues
  onSubmit: (values: ResourceFormValues) => void | Promise<void>
  /** Disables every control while the page is saving. */
  isLoading?: boolean
  /**
   * A node per name in `descriptor.form.extensions`, rendered where the
   * descriptor's `extensionPlacement` puts it (#1180). A name with no node is
   * simply not rendered — a page may fill one slot and not another.
   */
  extensions?: Readonly<Record<string, ReactNode>>
  /**
   * Replaces the shared `ResourceBodyEditor` the body control renders by
   * default — how a page opts into the prompt-only extensions (#914).
   */
  renderBody?: (props: BodySlotProps) => ReactNode
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataRequiredKeys?: string[]
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataReservedKeys?: string[]
  /** The form's handle (React 19 passes `ref` as a plain prop). */
  ref?: Ref<ResourceFormHandle>
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
  /**
   * Kind-specific header badges, as on the reading page (the prompt's Shared
   * badge), so they do not vanish on the way into edit.
   */
  headerExtra?: ReactNode
  /** Where Cancel goes. Called only once the unsaved-changes guard clears. */
  onCancel: () => void
  /** Forwarded to the Save action, for pages an e2e spec addresses by id. */
  saveTestId?: string
  /**
   * Disables Save without locking the form — a create page whose submit
   * cannot succeed yet (the memory page before the team has a project).
   */
  saveDisabled?: boolean
  /**
   * Unsaved state the form does not own — a page's `extensions` are its own
   * `useState` and are invisible to react-hook-form, so the memory's tags and
   * the prompt's MCP toggle would otherwise be discarded by Cancel or by a tab
   * close without any prompt.
   */
  extraDirty?: boolean
  /**
   * The resource the Attachments section is about. Omit while it is unknown
   * (no team resolved yet) — the section then renders nothing, as on the
   * reading page.
   */
  resource?: ResourceRef
  /**
   * The fetched resource, read by descriptor key for the rows that stay
   * read-only while editing — Created, Updated, a locked slug, the memory's
   * id, the blueprint's import source. Falls back to `initialValues`, which
   * for most pages already IS the fetched resource.
   */
  record?: Readonly<Record<string, unknown>>
  /** The Metadata section's Version row and history link, as on the reading page. */
  versionHistory?: VersionHistoryMeta
}

/** A form value as display text; anything that is not a string reads as absent. */
function valueOf(values: ResourceFormValues | undefined, key: string) {
  const value = new Map(Object.entries(values ?? {})).get(key)
  return typeof value === 'string' ? value : ''
}

/** "Create artifact" / "Save changes", and what it says while that runs. */
function saveActionLabel(
  descriptor: ResourceDescriptor,
  mode: ResourceFormMode,
  isLoading: boolean
) {
  if (!isLoading) return formSaveLabel(descriptor, mode)
  return mode === 'create' ? 'Creating…' : 'Saving…'
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
 * The extension slots (the prompt's MCP switch, the memory's tags), sorted
 * into where the descriptor places them — a Metadata row's value, or beside
 * the labels — which is where the reading page shows the same thing, so they
 * add no heading the reading page lacks (#1180). One with no placement goes at
 * the end of the Metadata section.
 */
function placeExtensions(
  descriptor: ResourceDescriptor,
  extensions: Readonly<Record<string, ReactNode>> | undefined,
  rowFieldNodes: ReadonlyMap<string, ReactNode>
) {
  const nodes = new Map(Object.entries(extensions ?? {}))
  const placement = new Map(
    Object.entries(descriptor.form?.extensionPlacement ?? {})
  )
  const rowControls = new Map(rowFieldNodes)
  const taxonomy: ReactNode[] = []
  const unplaced: ReactNode[] = []
  for (const name of descriptor.form?.extensions ?? []) {
    const node = nodes.get(name)
    if (!node) continue
    const place = placement.get(name)
    if (place?.section === 'details') {
      rowControls.set(place.row, node)
    } else {
      const target = place?.section === 'taxonomy' ? taxonomy : unplaced
      target.push(<div key={name}>{node}</div>)
    }
  }
  return { rowControls, taxonomy, unplaced }
}

/**
 * The generated form (#913) rendered in the reading shell (#916) — the one
 * create/edit layout since #1181 moved the four create pages onto it, so
 * creating a resource looks like editing it, which looks like reading it.
 *
 * View and edit are the same document, so they get the same layout: the body
 * editor takes the article slot at the identical reading measure and gutters, the
 * details column carries the reading page's own Metadata and Attachments
 * sections with the editable rows as inputs (#1180) — folding to the icon rail
 * from the same header toggle as on the detail page — and Save/Cancel are `ReadingAction`s, so they render as the
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
  headerExtra,
  onCancel,
  saveTestId,
  saveDisabled = false,
  extraDirty = false,
  ref,
  descriptor,
  mode,
  initialValues,
  onSubmit,
  isLoading = false,
  extensions,
  renderBody,
  metadataRequiredKeys,
  metadataReservedKeys,
  resource,
  record,
  versionHistory,
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
    getValues,
    isDirty,
    bodyNode,
    rowFieldNodes,
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

  useImperativeHandle(ref, () => ({ submit, getValues }), [submit, getValues])

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
        extra={headerExtra}
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

  const actions: ReadingAction[] = [
    {
      id: 'save',
      label: saveActionLabel(descriptor, mode, isLoading),
      icon: Save,
      emphasis: 'primary',
      disabled: isLoading || saveDisabled,
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

  const placed = placeExtensions(descriptor, extensions, rowFieldNodes)
  // What the rows and chips that stay read-only while editing read from.
  const readOnly = { ...initialValues, ...record }
  const formKeys = new Set(descriptor.form?.fields.map(spec => spec.key))

  // The reading page's column, section for section (#1180): the same ids,
  // headings, icons and order, with the editable rows as inputs in place and
  // the read-only facts (Created, Version, a locked slug) still as rows — so
  // View → Edit changes the values into controls and moves nothing.
  const metadata = RESOURCE_SECTION_CHROME.metadata
  const attachments = RESOURCE_SECTION_CHROME.attachments
  const sections: ReadingSection[] = [
    {
      id: RESOURCE_SECTION_IDS.metadata,
      label: metadata.label,
      icon: metadata.icon,
      content: (
        <div className="space-y-5" data-testid="resource-form-details">
          <ResourceMetadataSection
            descriptor={descriptor}
            resource={readOnly}
            versionHistory={versionHistory}
            controls={placed.rowControls}
          />
          {placed.unplaced}
          <TaxonomyPanel data-testid="resource-form-taxonomy">
            <ReadOnlyTaxonomyGroups
              descriptor={descriptor}
              resource={readOnly}
              editable={formKeys}
            />
            {taxonomyNode}
            {placed.taxonomy}
          </TaxonomyPanel>
        </div>
      ),
    },
    {
      id: RESOURCE_SECTION_IDS.attachments,
      label: attachments.label,
      icon: attachments.icon,
      content: resource && descriptor.capabilities.attachments && (
        <div className="space-y-2">
          <ResourceAttachments
            teamId={resource.teamId}
            ownerType={resource.kind}
            ownerId={resource.id}
          />
          {/* Attachments are not form state: an upload or a removal is saved
              the moment it happens, so Cancel cannot undo it and it never
              counts as an unsaved change. Say so where it happens. */}
          <p className="text-muted-foreground text-xs">
            Attachment changes save immediately — Cancel does not undo them.
          </p>
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
