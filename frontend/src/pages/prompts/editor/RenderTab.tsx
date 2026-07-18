import { AlertCircle } from 'lucide-react'

import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface RenderTabProps {
  allPlaceholders: string[]
  placeholderValues: Record<string, string>
  onPlaceholderChange: (placeholder: string, value: string) => void
  renderedBody: string
  renderError: string | null
  isRendering: boolean
}

export function RenderTab({
  allPlaceholders,
  placeholderValues,
  onPlaceholderChange,
  renderedBody,
  renderError,
  isRendering,
}: Readonly<RenderTabProps>) {
  return (
    <Card>
      <CardContent className="space-y-4 p-6">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">Render preview</h3>
          {isRendering && (
            <span className="text-muted-foreground text-sm">Rendering…</span>
          )}
        </div>

        {allPlaceholders.length > 0 && (
          <div className="bg-muted space-y-3 rounded-md p-4">
            <p className="text-sm font-medium">
              Fill in placeholder values
              <span className="text-muted-foreground ml-2 text-xs font-normal">
                (includes placeholders from referenced prompts)
              </span>
            </p>
            {allPlaceholders.map(placeholder => (
              <div key={placeholder} className="space-y-1">
                <Label className="text-xs">{placeholder}</Label>
                <Input
                  type="text"
                  value={placeholderValues[placeholder] ?? ''}
                  onChange={e => {
                    onPlaceholderChange(placeholder, e.target.value)
                  }}
                  placeholder={`Enter value for {{${placeholder}}}`}
                />
              </div>
            ))}
          </div>
        )}

        {renderError && (
          <Alert variant="destructive">
            <AlertCircle className="size-4" />
            <AlertDescription>{renderError}</AlertDescription>
          </Alert>
        )}

        <div className="prose dark:prose-invert bg-background min-h-96 max-w-none rounded-md border p-6">
          <MarkdownRenderer
            content={
              renderedBody ||
              'Enter placeholder values above to see the rendered prompt…'
            }
            enableCodeCopy={true}
            enableMermaid={true}
          />
        </div>
      </CardContent>
    </Card>
  )
}
