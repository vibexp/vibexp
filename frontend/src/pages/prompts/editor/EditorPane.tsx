import { AlertCircle, Download, Play, Wand2 } from 'lucide-react'

import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { PromptMentionTextarea } from '@/components/PromptMentionTextarea'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

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
}: EditorPaneProps) {
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

      <Tabs
        value={view}
        onValueChange={v => {
          onViewChange(v as EditorView)
        }}
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <TabsList>
            <TabsTrigger value="write">Write</TabsTrigger>
            <TabsTrigger value="preview">Preview</TabsTrigger>
            {isEditing && (
              <TabsTrigger value="render" disabled={isLoadingPlaceholders}>
                <Play className="mr-1 size-3.5" />
                Render
              </TabsTrigger>
            )}
          </TabsList>
          <div className="flex items-center gap-2">
            {!isEditing && (
              <Button variant="outline" size="sm" onClick={onLoadTemplateClick}>
                <Download className="mr-2 size-3.5" />
                Load template
              </Button>
            )}
            {view !== 'render' && (
              <Badge variant="outline" className="gap-1">
                <Wand2 className="size-3" />
                Type @ to reference prompts
              </Badge>
            )}
          </div>
        </div>

        <TabsContent value="write">
          <Card>
            <CardContent className="p-6">
              <PromptMentionTextarea
                data-testid="prompt-body-textarea"
                value={formData.body}
                onChange={onBodyChange}
                placeholder="Write your prompt here… Use markdown for **bold**, *italic*, `code`.&#10;&#10;💡 Type @ to reference other prompts"
                rows={30}
                error={errors.body}
                excludeCurrentPrompt={excludeCurrentPrompt}
                className="min-h-[600px]"
              />
              {errors.body && (
                <p className="text-destructive mt-2 flex items-center gap-1 text-sm">
                  <AlertCircle className="size-4" />
                  {errors.body}
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="preview">
          <Card>
            <CardContent className="min-h-[600px] p-6">
              <div className="prose dark:prose-invert max-w-none">
                <MarkdownRenderer
                  content={formData.body || 'No content to preview…'}
                  enableCodeCopy={true}
                  enableMermaid={true}
                />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {isEditing && (
          <TabsContent value="render">
            <RenderTab
              allPlaceholders={allPlaceholders}
              placeholderValues={placeholderValues}
              onPlaceholderChange={onPlaceholderChange}
              renderedBody={renderedBody}
              renderError={renderError}
              isRendering={isRendering}
            />
          </TabsContent>
        )}
      </Tabs>
    </div>
  )
}
