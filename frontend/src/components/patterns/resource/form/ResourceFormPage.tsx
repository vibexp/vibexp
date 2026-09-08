import { zodResolver } from '@hookform/resolvers/zod'
import type { ReactNode } from 'react'
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useForm } from 'react-hook-form'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

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

/** The submit handle the page header's Save button drives. */
export interface ResourceFormHandle {
  submit: () => void
}

export interface ResourceFormPageProps {
  descriptor: ResourceDescriptor
  mode: ResourceFormMode
  /** The resource being edited, as a value map. Omit when creating. */
  initialValues?: ResourceFormValues
  onSubmit: (values: ResourceFormValues) => void | Promise<void>
  /** Disables every control while the page is saving. */
  isLoading?: boolean
  /**
   * A node per name in `descriptor.form.extensions`, rendered under the
   * details card in the order the descriptor declares. A name with no node is
   * simply not rendered — a page may fill one slot and not another.
   */
  extensions?: Readonly<Record<string, ReactNode>>
  /** Replaces the default body textarea with a richer editor (#914). */
  renderBody?: (props: BodySlotProps) => ReactNode
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataRequiredKeys?: string[]
  /** Forwarded to the `metadata` control. Must be a stable reference. */
  metadataReservedKeys?: string[]
}

/**
 * The one create/edit page, generated from `descriptor.form` (#913).
 *
 * `ArtifactForm` (357 lines), `MemoryForm` (341), `BlueprintForm` (302) and
 * `PromptEditor` (361) each hand-wrote a zod schema, a slugifier and a card
 * layout for the same six or seven fields, in three different architectures —
 * so "add a field to a resource" was a four-file change and the four pages had
 * already diverged on validation, on which sidebar cards exist and on whether
 * the slug auto-fills at all. Which fields a resource edits is a property of
 * the resource type now, and this page is the only reader.
 *
 * There is no `switch (kind)` here and there must never be one: the switch is
 * over the closed `FormControl` union in `ResourceFormControl`, so a resource
 * type this build has never seen renders a working form.
 *
 * ## What this does NOT own
 *
 * The page header. Every create/edit route keeps its own `PageHeader` with the
 * Back and Save buttons, which is why the form exposes a `{ submit }` handle
 * through `forwardRef` — the pattern all three react-hook-form pages already
 * use. `formSaveLabel` / `formHeading` / `formSubtitle` derive that header's
 * wording from the same descriptor.
 *
 * Fetching, navigation and the request payload stay with the page too: this
 * hands `onSubmit` the parsed values, keyed by field key.
 */
export const ResourceFormPage = forwardRef<
  ResourceFormHandle,
  ResourceFormPageProps
>(function ResourceFormPage(
  {
    descriptor,
    mode,
    initialValues,
    onSubmit,
    isLoading = false,
    extensions,
    renderBody,
    metadataRequiredKeys,
    metadataReservedKeys,
  },
  ref
) {
  const formElRef = useRef<HTMLFormElement>(null)
  const slugManuallyEdited = useRef(mode === 'edit')
  const [metadataValid, setMetadataValid] = useState(true)

  const specs = useMemo(() => descriptor.form?.fields ?? [], [descriptor])
  const byKey = useMemo(() => formFieldsByKey(descriptor), [descriptor])
  const schema = useMemo(() => buildFormSchema(descriptor), [descriptor])
  const defaults = useMemo(
    () => defaultFormValues(descriptor, initialValues),
    [descriptor, initialValues]
  )

  const form = useForm<ResourceFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaults,
  })

  // Seeding happens on mount; an edit page that resolves its resource after
  // first paint re-seeds here, exactly as the three hand-written forms did.
  useEffect(() => {
    if (initialValues) form.reset(defaults)
  }, [initialValues, defaults, form])

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

  const handleSubmit = form.handleSubmit(async values => {
    // The metadata editor surfaces its own inline errors; block the submit so
    // an invalid map never reaches the API.
    if (!metadataValid) return
    await onSubmit(values)
  })

  useImperativeHandle(ref, () => ({
    submit() {
      formElRef.current?.requestSubmit()
    },
  }))

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
            <FormLabel className={spec.control === 'body' ? 'sr-only' : ''}>
              {label}
            </FormLabel>
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

  const sectionSpecs = (section: FormFieldSpec['section']) =>
    specs.filter(spec => spec.section === section)
  const bodySpecs = sectionSpecs('body')
  const detailSpecs = sectionSpecs('details')
  const taxonomySpecs = sectionSpecs('taxonomy')
  const extensionNodes = new Map(Object.entries(extensions ?? {}))

  return (
    <Form {...form}>
      <form
        ref={formElRef}
        data-testid="resource-form"
        className="grid gap-6 lg:grid-cols-3"
        onSubmit={event => {
          void handleSubmit(event)
        }}
      >
        <div className="min-w-0 space-y-4 lg:col-span-2">
          {bodySpecs.map(renderSpec)}
        </div>
        <div className="min-w-0 space-y-4">
          {detailSpecs.length > 0 && (
            <Card data-testid="resource-form-details">
              <CardHeader>
                <CardTitle className="text-sm">Details</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {detailSpecs.map(renderSpec)}
              </CardContent>
            </Card>
          )}
          {(descriptor.form?.extensions ?? []).map(name => {
            const node = extensionNodes.get(name)
            return node ? <div key={name}>{node}</div> : null
          })}
          {taxonomySpecs.length > 0 && (
            <Card data-testid="resource-form-taxonomy">
              <CardHeader>
                <CardTitle className="text-sm">Labels &amp; metadata</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {taxonomySpecs.map(renderSpec)}
              </CardContent>
            </Card>
          )}
        </div>
      </form>
    </Form>
  )
})
