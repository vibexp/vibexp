import { Copy, Eye, FileCode } from 'lucide-react'

import { LoadingSpinner } from '@/components/LoadingSpinner'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Prompt } from '@/types'

interface Props {
  prompt: Prompt
  tab: 'rendered' | 'raw'
  onTabChange: (tab: 'rendered' | 'raw') => void
  renderedBody: string
  renderError: string | null
  isRendering: boolean
  isLoadingPlaceholders: boolean
  allPlaceholders: string[]
  placeholderValues: Record<string, string>
  updatePlaceholderValue: (placeholder: string, value: string) => void
  onCopy: () => void
}

export function PromptContentCard({
  prompt,
  tab,
  onTabChange,
  renderedBody,
  renderError,
  isRendering,
  isLoadingPlaceholders,
  allPlaceholders,
  placeholderValues,
  updatePlaceholderValue,
  onCopy,
}: Props) {
  const body = renderedBody !== '' ? renderedBody : prompt.body

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Content</CardTitle>
        <Button variant="outline" size="sm" onClick={onCopy}>
          <Copy className="mr-2 size-4" />
          Copy
        </Button>
      </CardHeader>
      <CardContent>
        <Tabs
          value={tab}
          onValueChange={v => {
            onTabChange(v as 'rendered' | 'raw')
          }}
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="rendered">
              <Eye className="mr-2 size-4" />
              Rendered
            </TabsTrigger>
            <TabsTrigger value="raw">
              <FileCode className="mr-2 size-4" />
              Raw
            </TabsTrigger>
          </TabsList>

          <TabsContent value="rendered" className="space-y-4">
            {allPlaceholders.length > 0 && (
              <div className="bg-muted/40 space-y-2 rounded-md border p-3">
                <div className="text-xs font-medium">Placeholders</div>
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {allPlaceholders.map(ph => (
                    <div key={ph} className="space-y-1">
                      <label className="text-muted-foreground text-xs font-medium">
                        {ph}
                      </label>
                      <Input
                        value={placeholderValues[ph] ?? ''}
                        onChange={e => {
                          updatePlaceholderValue(ph, e.target.value)
                        }}
                        placeholder={`Enter ${ph}`}
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}
            {renderError && (
              <Alert variant="destructive">
                <AlertTitle>Render error</AlertTitle>
                <AlertDescription>{renderError}</AlertDescription>
              </Alert>
            )}
            {isRendering || isLoadingPlaceholders ? (
              <LoadingSpinner label="Rendering…" />
            ) : (
              <MarkdownRenderer content={body} syntaxTheme="auto" />
            )}
          </TabsContent>

          <TabsContent value="raw">
            <pre className="bg-muted text-muted-foreground overflow-x-auto rounded-md p-4 font-mono text-xs whitespace-pre-wrap">
              {prompt.body}
            </pre>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}
