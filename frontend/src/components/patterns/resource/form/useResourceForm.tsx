import { zodResolver } from '@hookform/resolvers/zod'
import type { FormEvent, ReactNode, RefObject } from 'react'
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
   * Called when a submit attempt fails validation. The reading-shell layout
   * uses it to reopen a folded details column, where most fields live — an
   * error the reader cannot see is an error they cannot fix (#916).
   */
  onInvalidSubmit?: () => void
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
  /** Controls of `section: 'details'`, or null when the kind declares none. */
  detailsNode: ReactNode
  /** Controls of `section: 'taxonomy'`, or null when the kind declares none. */
  taxonomyNode: ReactNode
}

/**
 * The generated create/edit form (#913), as slots rather than as a layout.
 *
 * One form context, two layouts (#916): `ResourceFormPage` renders these nodes
 * as the standalone card grid, and `ResourceFormReadingPage` renders the same
 * nodes into the reading shell's article and details column. Everything that
 * makes the form behave — the zod schema, the content-keyed re-seed, the slug
 * auto-fill, the metadata validity gate — lives here exactly once, so the two
 * layouts cannot drift into two form implementations.
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
        onInvalidSubmit?.()
        return
      }
      await onSubmit(values)
    },
    () => {
      onInvalidSubmit?.()
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

  const sectionNodes = (section: FormFieldSpec['section']) => {
    const matching = specs.filter(spec => spec.section === section)
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
  }
}
