import { AlertCircle } from 'lucide-react'

import type { ResourceBodyEditorExtensions } from '@/components/patterns/resource'
import { ResourceBodyEditor } from '@/components/patterns/resource'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { RenderTab } from './RenderTab'
import type { EditorView, PromptFormData } from './types'

interface EditorPaneProps {
  formData: PromptFormData
  errors: Partial<PromptFormData>
  view: EditorView
  onViewChange: (view: EditorView) => void
  onNameChange: (name: string) => void
  onBodyChange: (body: string) => void
  isEditing: boolean
  isLoadingPlaceholders: boolean
  excludeCurrentPrompt?: string
  onLoadTemplateClick: () => void
  // Render-tab props
  allPlaceholders: string[]
  placeholderValues: Record<string, string>
  onPlaceholderChange: (placeholder: string, value: string) => void
  renderedBody: string
  renderError: string | null
  isRendering: boolean
}

/**
 * The prompt editor's left column: the name card, then the shared body editor.
 *
 * Everything below the name card used to be written out here — the tab shell,
 * the mention textarea, the preview pane, the Render tab and the template
 * button — which is exactly why no other resource could have a preview. It is
 * now `ResourceBodyEditor` plus the three extensions that really are
 * prompt-only (#914).
 */
export function EditorPane({
  formData,
  errors,
  view,
  onViewChange,
  onNameChange,
  onBodyChange,
  isEditing,
  isLoadingPlaceholders,
  excludeCurrentPrompt,
  onLoadTemplateClick,
  allPlaceholders,
  placeholderValues,
  onPlaceholderChange,
  renderedBody,
  renderError,
  isRendering,
}: Readonly<EditorPaneProps>) {
  // Rendering is only meaningful once the prompt exists (its placeholders are
  // resolved server-side), and the template loader only while creating one —
  // so both tabs come and go, and the editor tolerates that by construction.
  const extensions: ResourceBodyEditorExtensions = {
    mentions: { excludeCurrentPrompt },
    render: isEditing
      ? {
          disabled: isLoadingPlaceholders,
          content: (
            <RenderTab
              allPlaceholders={allPlaceholders}
              placeholderValues={placeholderValues}
              onPlaceholderChange={onPlaceholderChange}
              renderedBody={renderedBody}
              renderError={renderError}
              isRendering={isRendering}
            />
          ),
        }
      : undefined,
    templates: isEditing ? undefined : onLoadTemplateClick,
  }

  return (
    <div className="flex-1 space-y-4 lg:w-[70%]">
      <Card>
        <CardContent className="space-y-3 p-6">
          <div className="space-y-1.5">
            <Label htmlFor="prompt-name">
              Name <span className="text-destructive">*</span>
            </Label>
            <Input
              id="prompt-name"
              type="text"
              data-testid="prompt-name-input"
              value={formData.name}
              onChange={e => {
                onNameChange(e.target.value)
              }}
              placeholder="Enter prompt name"
              aria-invalid={!!errors.name}
            />
          </div>
          {errors.name && (
            <p className="text-destructive flex items-center gap-1 text-sm">
              <AlertCircle className="size-4" />
              {errors.name}
            </p>
          )}
          {formData.slug && (
            <p className="text-muted-foreground text-sm">
              <span className="font-medium">Slug:</span>{' '}
              <span className="font-mono text-xs">{formData.slug}</span>
            </p>
          )}
        </CardContent>
      </Card>

      <ResourceBodyEditor
        data-testid="prompt-body-textarea"
        value={formData.body}
        onChange={onBodyChange}
        view={view}
        onViewChange={onViewChange}
        error={errors.body}
        placeholder="Write your prompt here… Use markdown for **bold**, *italic*, `code`.&#10;&#10;💡 Type @ to reference other prompts"
        extensions={extensions}
      />
    </div>
  )
}
