import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router'

import { ReadingPage } from '@/components/patterns/reading-page'
import type {
  BodySlotProps,
  ResourceBodyEditorExtensions,
  ResourceFormHandle,
  ResourceFormValues,
} from '@/components/patterns/resource'
import {
  formHeading,
  getResourceDescriptor,
  ResourceBodyEditor,
  ResourceFormReadingPage,
} from '@/components/patterns/resource'
import { PromptTemplateLoader } from '@/components/PromptTemplateLoader'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useAnalytics } from '@/hooks'
import { toast } from '@/lib/toast'
import { toPromptRequest } from '@/pages/prompts/promptRequest'
import { projectService } from '@/services/projectService'
import type { Prompt } from '@/services/promptService'
import { promptService } from '@/services/promptService'
import { ANALYTICS_EVENTS } from '@/types/analytics'
import { getErrorMessage } from '@/utils/errorHandling'

import { McpExposureCard } from './editor/McpExposureCard'
import { RenderTab } from './editor/RenderTab'
import type { EditorView } from './editor/types'
import { usePromptSave } from './editor/usePromptSave'
import { useRenderPreview } from './editor/useRenderPreview'
import { SharedBadge } from './SharedBadge'

const descriptor = getResourceDescriptor('prompt')

/** What "remix this gallery prompt" navigates here with. */
interface PrefilledPrompt {
  title?: string
  body?: string
  description?: string
}

function prefilledValues(prefilled: PrefilledPrompt | null) {
  if (!prefilled) return {}
  return {
    name: prefilled.title ? `Based on: ${prefilled.title}` : '',
    description: prefilled.description ?? '',
    body: prefilled.body ?? '',
  }
}

/**
 * Create and edit a prompt, on the shared `ResourceFormReadingPage` (#915,
 * #1181 for create).
 *
 * The three things that really are prompt-only survive as extensions rather
 * than as a second form architecture: the body editor's `@`-mention textarea,
 * its Render tab, and the template loader (#914), plus the `mcp-exposure` slot
 * the descriptor declares. Everything the page used to hand-write — a
 * `PromptFormData` state object, a `validate()`, a slugifier, and a whole
 * settings pane of `Label`/`Input` pairs that never used the `Form` primitives
 * — is descriptor data now.
 */
export function PromptEditor() {
  const navigate = useNavigate()
  const location = useLocation()
  const { slug } = useParams<{ slug: string }>()
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { trackEvent } = useAnalytics()

  const prefilledData = location.state as PrefilledPrompt | null

  const [loading, setLoading] = useState(!!slug)
  // Create mode waits for the projects fetch before painting the form. Seeding
  // `initialValues` after first paint is a `reset`, and a reset discards
  // everything typed in the meantime — the trap `useResourceForm` documents.
  const [loadingProjects, setLoadingProjects] = useState(!slug)
  const [view, setView] = useState<EditorView>('write')
  const [showTemplateLoader, setShowTemplateLoader] = useState(false)
  const [prompt, setPrompt] = useState<Prompt | null>(null)
  const [defaultProjectId, setDefaultProjectId] = useState('')
  const [mcpExpose, setMcpExpose] = useState(false)
  const [templateValues, setTemplateValues] =
    useState<ResourceFormValues | null>(null)
  const formRef = useRef<ResourceFormHandle>(null)

  const isEditing = !!slug
  const mode = isEditing ? 'edit' : 'create'

  const { saving, save } = usePromptSave({
    teamId: currentTeam?.id,
    prompt,
    trackEvent,
  })
  const {
    allPlaceholders,
    placeholderValues,
    setPlaceholderValue,
    renderedBody,
    renderError,
    isRendering,
    isLoadingPlaceholders,
    fetchAllPlaceholders,
  } = useRenderPreview({
    teamId: currentTeam?.id,
    prompt,
    view,
    isEditing,
  })

  const loadPrompt = useCallback(
    async (promptSlug: string) => {
      if (!currentTeam) {
        toast.error('No team selected')
        return
      }
      try {
        setLoading(true)
        const p = await promptService.getPrompt(currentTeam.id, promptSlug)
        setPrompt(p)
        setMcpExpose(p.mcp_expose)
      } catch (error) {
        toast.error(getErrorMessage(error, 'Failed to load prompt'))
        void navigate('/prompts')
      } finally {
        setLoading(false)
      }
    },
    [navigate, currentTeam]
  )

  useEffect(() => {
    if (isEditing) {
      setLoadingProjects(false)
      return
    }
    // Still resolving the team: stay on the skeleton rather than painting a
    // form that is about to be re-seeded.
    if (isLoadingTeam) return
    if (!currentTeam) {
      setLoadingProjects(false)
      return
    }
    const fetchProjects = async () => {
      try {
        const response = await projectService.getProjects(currentTeam.id, {})
        // A team with exactly one project preselects it. `ProjectPicker` has no
        // default of its own (#1790), and this is the default the prompt e2e
        // journeys create prompts through without touching the picker.
        if (response.projects.length === 1) {
          setDefaultProjectId(response.projects[0].id)
        }
      } catch {
        setDefaultProjectId('')
      } finally {
        setLoadingProjects(false)
      }
    }
    void fetchProjects()
  }, [currentTeam, isEditing, isLoadingTeam])

  useEffect(() => {
    if (slug && !isLoadingTeam) {
      void loadPrompt(slug)
    }
  }, [slug, loadPrompt, isLoadingTeam])

  // Content-stable: the form re-seeds whenever these values differ
  // from the last ones, so a loaded prompt, a preselected project and a loaded
  // template each reach the form exactly once.
  const initialValues = useMemo<ResourceFormValues | undefined>(() => {
    // A loaded template already carries the form's other values (see
    // `handleLoadTemplate`), so it replaces the seed rather than merging.
    if (templateValues) return templateValues
    if (prompt) return prompt
    const seeded: ResourceFormValues = prefilledValues(prefilledData)
    if (defaultProjectId) seeded.project_id = defaultProjectId
    return Object.keys(seeded).length > 0 ? seeded : undefined
  }, [prompt, defaultProjectId, templateValues, prefilledData])

  const handleSubmit = async (values: ResourceFormValues) => {
    const savedSlug = await save(toPromptRequest(values, mcpExpose))
    if (savedSlug) {
      void navigate(`/prompts/${savedSlug}`)
    }
  }

  const handleLoadTemplate = (templatePrompt: Prompt) => {
    const current = formRef.current?.getValues() ?? {}
    const currentBody = typeof current.body === 'string' ? current.body : ''
    if (
      currentBody.trim() &&
      !window.confirm(
        'Loading a template will replace your current content. Continue?'
      )
    ) {
      return
    }
    const currentName = typeof current.name === 'string' ? current.name : ''
    // Re-seeding is a `reset`, so it must carry the fields the template does
    // NOT set — the project already picked, the status, the labels — or loading
    // a template would quietly clear them.
    setTemplateValues({
      ...current,
      name: currentName || `${templatePrompt.name} (Copy)`,
      description: templatePrompt.description,
      body: templatePrompt.body,
    })
    toast.success(`Template "${templatePrompt.name}" loaded`)
  }

  const handleViewChange = async (next: EditorView) => {
    setView(next)
    const basePayload = {
      prompt_id: slug ?? 'new',
      prompt_title: prompt?.name ?? 'Untitled',
      action_context: 'preview' as const,
    }
    if (next === 'preview') {
      trackEvent({
        event: ANALYTICS_EVENTS.PROMPT_PREVIEW_VIEWED,
        properties: basePayload,
      })
    }
    if (next === 'render' && slug) {
      await fetchAllPlaceholders()
      trackEvent({
        event: ANALYTICS_EVENTS.PROMPT_PREVIEW_VIEWED,
        properties: { ...basePayload, preview_type: 'render' },
      })
    }
  }

  // Rendering is only meaningful once the prompt exists (its placeholders are
  // resolved server-side), and the template loader only while creating one — so
  // both tabs come and go, and the editor tolerates that by construction.
  const bodyExtensions: ResourceBodyEditorExtensions = {
    mentions: { excludeCurrentPrompt: prompt?.slug },
    render: isEditing
      ? {
          disabled: isLoadingPlaceholders,
          content: (
            <RenderTab
              allPlaceholders={allPlaceholders}
              placeholderValues={placeholderValues}
              onPlaceholderChange={setPlaceholderValue}
              renderedBody={renderedBody}
              renderError={renderError}
              isRendering={isRendering}
            />
          ),
        }
      : undefined,
    templates: isEditing
      ? undefined
      : () => {
          setShowTemplateLoader(true)
        },
  }

  const renderBody = (props: BodySlotProps) => (
    <ResourceBodyEditor
      {...props}
      view={view}
      onViewChange={v => {
        void handleViewChange(v)
      }}
      extensions={bodyExtensions}
    />
  )

  if (loading || loadingProjects) {
    // The skeleton renders inside the reading shell, so the layout is in place
    // before the fetch resolves rather than arriving with the prompt.
    return (
      <ReadingPage
        title={formHeading(descriptor, mode)}
        description={isEditing ? 'Loading prompt…' : 'Loading projects…'}
        presentation="editing"
      >
        <Skeleton className="h-64 w-full" />
      </ReadingPage>
    )
  }

  // Creating a prompt is the same document as editing it, which is the same
  // document as reading it, so both go through the reading shell (#916,
  // #1181). The template loader only exists while creating.
  return (
    <>
      <ResourceFormReadingPage
        ref={formRef}
        title={formHeading(descriptor, mode)}
        updatedAt={prompt?.updated_at}
        headerExtra={prompt?.is_shared && <SharedBadge />}
        descriptor={descriptor}
        mode={mode}
        initialValues={initialValues}
        onSubmit={handleSubmit}
        isLoading={saving}
        renderBody={renderBody}
        saveTestId="prompt-save-button"
        // The MCP card is page state, invisible to react-hook-form: without
        // this, toggling it and hitting Cancel discards it with no prompt.
        extraDirty={mcpExpose !== (prompt?.mcp_expose ?? false)}
        extensions={{
          'mcp-exposure': (
            <McpExposureCard
              value={mcpExpose}
              onChange={setMcpExpose}
              disabled={saving}
            />
          ),
        }}
        onCancel={() => {
          void navigate(prompt ? `/prompts/${prompt.slug}` : '/prompts')
        }}
      />

      {!isEditing && (
        <PromptTemplateLoader
          isOpen={showTemplateLoader}
          onClose={() => {
            setShowTemplateLoader(false)
          }}
          onSelectPrompt={handleLoadTemplate}
          excludeCurrentPrompt={prompt?.slug}
        />
      )}
    </>
  )
}
