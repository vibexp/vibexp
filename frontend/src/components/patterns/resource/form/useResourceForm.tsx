import { zodResolver } from '@hookform/resolvers/zod'
import type { FormEvent, KeyboardEvent, ReactNode, RefObject } from 'react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useForm } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { labelVariants } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import { fieldOfRole } from '../fieldOfRole'
import type { FormFieldSpec, ResourceDescriptor } from '../types'
import type { ResourceFormValues } from './buildFormSchema'
import {
  buildFormSchema,
  defaultFormValues,
  formFieldLabel,
  formFieldsByKey,
  slugify,
} from './buildFormSchema'
import type { ResourceFormMode } from './formLabels'
import { InlineTextarea } from './InlineTextarea'
import type { BodySlotProps } from './ResourceFormControl'
import { ResourceFormControl } from './ResourceFormControl'

export interface UseResourceFormOptions {
  descriptor: ResourceDescriptor
  mode: ResourceFormMode
  /** The resource being edited, as a value map. Omit when creating. */
  initialValues?: ResourceFormValues
  onSubmit: (values: ResourceFormValues) => void | Promise<void>
  /** Disables every control while the page is saving. */
  isLoading?: boolean
  /** Replaces the shared `ResourceBodyEditor` the body control renders (#914). */
  renderBody?: (props: BodySlotProps) => ReactNode
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataRequiredKeys?: string[]
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataReservedKeys?: string[]
  /**
   * Called when a submit attempt fails validation, with the keys of the fields
   * that failed. The reading-shell layout uses it to reopen a folded details
   * column, where most fields live — an error the reader cannot see is an
   * error they cannot fix (#916).
   */
  onInvalidSubmit?: (invalidKeys: readonly string[]) => void
}

/** The generated form, as nodes a layout places wherever it wants. */
export interface ResourceFormSlots {
  form: UseFormReturn<ResourceFormValues>
  /** Attach to the `<form>` element the layout renders. */
  formElRef: RefObject<HTMLFormElement | null>
  /** `onSubmit` handler for that element. */
  onFormSubmit: (event: FormEvent<HTMLFormElement>) => void
  /** Submit from outside the form element (a Save button in a header or rail). */
  submit: () => void
  getValues: () => ResourceFormValues
  /** Whether any field differs from the seeded values. */
  isDirty: boolean
  /** Controls of `section: 'body'`. */
  bodyNode: ReactNode
  /**
   * Controls of `section: 'details'`, or null when the kind declares none.
   * Excludes the header fields, which render as `titleNode` / `summaryNode`.
   */
  detailsNode: ReactNode
  /** The inline name input, or null when the kind has none. */
  titleNode: ReactNode
  /** The inline summary input, or null when the kind has none. */
  summaryNode: ReactNode
  /** Keys of the fields rendered in the header rather than the details column. */
  headerKeys: readonly string[]
  /** Controls of `section: 'taxonomy'`, or null when the kind declares none. */
  taxonomyNode: ReactNode
}

/**
 * The inline header inputs (#1179). Borderless, so at rest they look exactly
 * like the reading page's heading and lead; a token fill on hover/focus is the
 * only sign they are editable. `-mx-2 px-2` keeps the TEXT on the reading
 * page's x while giving the fill some room, and the widened box gives back the
 * 1rem the negative margins take, so the text wraps at the same width too.
 */
const INLINE_FIELD_CLASS =
  'block -mx-2 w-[calc(100%+1rem)] resize-none overflow-hidden rounded-md border-0 bg-transparent px-2 py-0 field-sizing-content outline-none transition-colors placeholder:text-muted-foreground/60 hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring aria-invalid:ring-2 aria-invalid:ring-destructive disabled:cursor-not-allowed disabled:opacity-50'

/** The reading page's `<h1>` typography. */
const INLINE_TITLE_CLASS = 'text-2xl font-semibold tracking-tight'

/** The reading page's lead typography, at the same prose measure (+1rem, as above). */
const INLINE_SUMMARY_CLASS =
  'max-w-[calc(var(--container-3xl)+1rem)] text-sm text-muted-foreground'

type HeaderSlot = 'title' | 'summary'

/**
 * A heading is one line of text: Enter must not insert a newline (it would be
 * saved into a single-line field), and a pasted newline becomes a space. The
 * title is still a `<textarea>` so a long name wraps on a phone, which an
 * `<input>` cannot do.
 */
function blockEnter(event: KeyboardEvent<HTMLTextAreaElement>) {
  if (event.key === 'Enter') event.preventDefault()
}

/**
 * The generated create/edit form (#913), as slots rather than as a layout.
 *
 * The layout is `ResourceFormReadingPage`, which renders these nodes into the
 * reading shell's header, article and details column (#916, #1179; #1181
 * retired the standalone card grid that was the second layout). Everything that
 * makes the form behave — the zod schema, the content-keyed re-seed, the slug
 * auto-fill, the metadata validity gate — lives here exactly once, so create
 * and edit cannot drift into two form implementations.
 *
 * There is no `switch (kind)` here and there must never be one: the switch is
 * over the closed `FormControl` union in `ResourceFormControl`.
 */
export function useResourceForm({
  descriptor,
  mode,
  initialValues,
  onSubmit,
  isLoading = false,
  renderBody,
  metadataRequiredKeys,
  metadataReservedKeys,
  onInvalidSubmit,
}: UseResourceFormOptions): ResourceFormSlots {
  const formElRef = useRef<HTMLFormElement>(null)
  const slugManuallyEdited = useRef(mode === 'edit')
  const [metadataValid, setMetadataValid] = useState(true)

  const specs = useMemo(() => descriptor.form?.fields ?? [], [descriptor])
  const byKey = useMemo(() => formFieldsByKey(descriptor), [descriptor])
  const schema = useMemo(() => buildFormSchema(descriptor), [descriptor])

  const form = useForm<ResourceFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultFormValues(descriptor, initialValues),
  })

  // Re-seeding is keyed on the CONTENT of `initialValues`, never on its
  // identity: a page builds it as an object literal from the fetched resource,
  // so an identity check re-seeds on every parent render — and a re-seed is a
  // `reset`, which silently discards everything typed since. The page's own
  // extension slots are page-owned state, so those re-renders are certain. An
  // edit page whose resource resolves after first paint still re-seeds,
  // because that is a genuine content change.
  const seed = JSON.stringify(initialValues ?? null)
  const lastSeed = useRef(seed)
  useEffect(() => {
    if (!initialValues || lastSeed.current === seed) return
    lastSeed.current = seed
    form.reset(defaultFormValues(descriptor, initialValues))
  }, [seed, initialValues, descriptor, form])

  const nameField = fieldOfRole(descriptor, 'name')
  const slugSpec = specs.find(spec => spec.pattern === 'slug')
  const metadataSpec = specs.find(spec => spec.control === 'metadata')

  // The header fields (#1179) — the name and summary render as the page's own
  // header, inline in the heading's typography — read off the descriptor's
  // roles: a kind with no `name` text field or no `summary` textarea simply
  // keeps that one in the column.
  const summaryField = fieldOfRole(descriptor, 'summary')
  const titleSpec = specs.find(
    spec => spec.key === nameField?.key && spec.control === 'text'
  )
  const summarySpec = specs.find(
    spec => spec.key === summaryField?.key && spec.control === 'textarea'
  )
  const headerSpecs = [titleSpec, summarySpec].filter(
    (spec): spec is FormFieldSpec => spec !== undefined
  )
  // A key no field owns simply watches nothing, which is the right answer for
  // a kind with no name field.
  const nameValue = form.watch(nameField?.key ?? '__no_name_field__')

  // On create, the slug tracks the name until the user edits the slug field —
  // the behaviour the artifact and prompt pages had and the blueprint page
  // never got.
  useEffect(() => {
    if (mode !== 'create' || !slugSpec || !nameField) return
    if (slugManuallyEdited.current) return
    const next = slugify(typeof nameValue === 'string' ? nameValue : '')
    form.setValue(slugSpec.key, next, { shouldValidate: next.length > 0 })
  }, [nameValue, mode, slugSpec, nameField, form])

  const handleSubmit = form.handleSubmit(
    async values => {
      // The metadata editor surfaces its own inline errors; block the submit so
      // an invalid map never reaches the API.
      if (!metadataValid) {
        onInvalidSubmit?.(metadataSpec ? [metadataSpec.key] : [])
        return
      }
      await onSubmit(values)
    },
    errors => {
      onInvalidSubmit?.(Object.keys(errors))
    }
  )

  const renderSpec = (spec: FormFieldSpec) => {
    const label = formFieldLabel(byKey, spec.key)
    const locked = isLoading || (spec.editableOnCreateOnly && mode === 'edit')
    return (
      <FormField
        key={spec.key}
        control={form.control}
        name={spec.key}
        render={({ field }) => (
          <FormItem>
            {/*
              A `FormLabel` is a real `<label for=…>`, so it needs a labelable
              leaf to point at. The metadata editor is a list of its own
              labelled rows, not one control — labelling it would leave the
              `for` dangling, which is worse than no label at all.
            */}
            {spec.control === 'metadata' ? (
              // Styled from the same source as a real label, so the heading
              // cannot drift from the ones beside it.
              <p className={labelVariants()}>{label}</p>
            ) : (
              <FormLabel className={spec.control === 'body' ? 'sr-only' : ''}>
                {label}
              </FormLabel>
            )}
            <FormControl>
              <ResourceFormControl
                spec={spec}
                field={byKey.get(spec.key)}
                label={label}
                resourceType={descriptor.plural}
                value={field.value}
                disabled={!!locked}
                metadataRequiredKeys={metadataRequiredKeys}
                metadataReservedKeys={metadataReservedKeys}
                onMetadataValidityChange={setMetadataValid}
                renderBody={renderBody}
                onChange={next => {
                  if (spec === slugSpec) slugManuallyEdited.current = true
                  field.onChange(next)
                }}
              />
            </FormControl>
            {spec.description && (
              <FormDescription>{spec.description}</FormDescription>
            )}
            <FormMessage />
          </FormItem>
        )}
      />
    )
  }

  const renderHeaderSpec = (spec: FormFieldSpec, slot: HeaderSlot) => {
    const label = formFieldLabel(byKey, spec.key)
    const locked = isLoading || (spec.editableOnCreateOnly && mode === 'edit')
    return (
      <FormField
        key={spec.key}
        control={form.control}
        name={spec.key}
        render={({ field }) => (
          // `space-y-0`: the sr-only label is still a child, and FormItem's
          // default spacing would push the input off the heading's y.
          <FormItem className="space-y-0">
            <FormLabel className="sr-only">{label}</FormLabel>
            <FormControl>
              <InlineTextarea
                value={typeof field.value === 'string' ? field.value : ''}
                disabled={!!locked}
                maxLength={spec.maxLength}
                placeholder={
                  slot === 'title'
                    ? `Untitled ${descriptor.singular}`
                    : 'Add a description…'
                }
                data-testid={spec.testId}
                className={cn(
                  INLINE_FIELD_CLASS,
                  slot === 'title' ? INLINE_TITLE_CLASS : INLINE_SUMMARY_CLASS
                )}
                onKeyDown={slot === 'title' ? blockEnter : undefined}
                onBlur={field.onBlur}
                onChange={event => {
                  const next = event.target.value
                  field.onChange(
                    slot === 'title' ? next.replace(/\r?\n/g, ' ') : next
                  )
                }}
              />
            </FormControl>
            <FormMessage className="mt-1" />
          </FormItem>
        )}
      />
    )
  }

  const sectionNodes = (section: FormFieldSpec['section']) => {
    const matching = specs.filter(
      spec => spec.section === section && !headerSpecs.includes(spec)
    )
    return matching.length > 0 ? matching.map(renderSpec) : null
  }

  return {
    form,
    formElRef,
    onFormSubmit: event => {
      void handleSubmit(event)
    },
    submit: () => {
      formElRef.current?.requestSubmit()
    },
    getValues: () => form.getValues(),
    isDirty: form.formState.isDirty,
    bodyNode: sectionNodes('body'),
    detailsNode: sectionNodes('details'),
    taxonomyNode: sectionNodes('taxonomy'),
    titleNode: titleSpec ? renderHeaderSpec(titleSpec, 'title') : null,
    summaryNode: summarySpec ? renderHeaderSpec(summarySpec, 'summary') : null,
    headerKeys: headerSpecs.map(spec => spec.key),
  }
}
