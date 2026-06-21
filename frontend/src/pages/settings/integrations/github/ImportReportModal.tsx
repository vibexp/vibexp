import { AlertCircle, CheckCircle, XCircle } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { BlueprintImportReport } from '@/types/github'

interface ImportReportModalProps {
  isOpen: boolean
  report: BlueprintImportReport | null
  repositoryName: string
  onClose: () => void
}

type TabType = 'success' | 'failed' | 'skipped'

export function ImportReportModal({
  isOpen,
  report,
  repositoryName,
  onClose,
}: ImportReportModalProps) {
  const [activeTab, setActiveTab] = useState<TabType>('success')

  useEffect(() => {
    if (isOpen) {
      setActiveTab('success')
    }
  }, [isOpen, report])

  if (!report) return null

  return (
    <Dialog
      open={isOpen}
      onOpenChange={open => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Blueprint Import Report</DialogTitle>
        </DialogHeader>

        <div className="bg-muted rounded-md p-3">
          <h3 className="mb-2 text-sm font-medium">
            Import Summary for {repositoryName}
          </h3>
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div>
              <span className="text-muted-foreground">Total scanned:</span>{' '}
              <span className="font-medium">{report.total_scanned} files</span>
            </div>
            <div>
              <span className="text-muted-foreground">Successful:</span>{' '}
              <span className="font-medium text-success">
                {report.total_successful} blueprints
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Failed:</span>{' '}
              <span className="font-medium text-destructive">
                {report.total_failed} errors
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Skipped:</span>{' '}
              <span className="font-medium text-warning">
                {report.total_skipped} files
              </span>
            </div>
          </div>
        </div>

        <Tabs
          value={activeTab}
          onValueChange={value => {
            setActiveTab(value as TabType)
          }}
        >
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="success">
              Success ({report.total_successful})
            </TabsTrigger>
            <TabsTrigger value="failed">
              Failed ({report.total_failed})
            </TabsTrigger>
            <TabsTrigger value="skipped">
              Skipped ({report.total_skipped})
            </TabsTrigger>
          </TabsList>

          <TabsContent value="success">
            <ScrollArea className="h-72">
              {report.successful_items.length === 0 ? (
                <p className="text-muted-foreground py-4 text-center text-sm">
                  No blueprints were successfully imported
                </p>
              ) : (
                <div className="space-y-2">
                  {report.successful_items.map((item, index) => (
                    <div
                      key={index}
                      className="rounded-md border border-success/20 bg-success/5 p-3"
                    >
                      <div className="flex items-start gap-3">
                        <CheckCircle className="mt-0.5 size-5 shrink-0 text-success" />
                        <div className="flex-1">
                          <p className="text-sm font-medium">{item.title}</p>
                          <p className="text-muted-foreground mt-0.5 text-xs">
                            {item.file_path}
                          </p>
                          <div className="mt-2 flex items-center gap-2">
                            <Badge variant="secondary">{item.type}</Badge>
                            {item.subtype && (
                              <Badge variant="secondary">{item.subtype}</Badge>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </ScrollArea>
          </TabsContent>

          <TabsContent value="failed">
            <ScrollArea className="h-72">
              {report.failed_items.length === 0 ? (
                <p className="text-muted-foreground py-4 text-center text-sm">
                  No errors occurred during import
                </p>
              ) : (
                <div className="space-y-2">
                  {report.failed_items.map((item, index) => (
                    <div
                      key={index}
                      className="rounded-md border border-destructive/20 bg-destructive/5 p-3"
                    >
                      <div className="flex items-start gap-3">
                        <XCircle className="mt-0.5 size-5 shrink-0 text-destructive" />
                        <div className="flex-1">
                          <p className="text-sm font-medium">
                            {item.file_path}
                          </p>
                          <p className="text-muted-foreground mt-0.5 text-xs">
                            {item.error}
                          </p>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </ScrollArea>
          </TabsContent>

          <TabsContent value="skipped">
            <ScrollArea className="h-72">
              {report.skipped_items.length === 0 ? (
                <p className="text-muted-foreground py-4 text-center text-sm">
                  No files were skipped
                </p>
              ) : (
                <div className="space-y-2">
                  {report.skipped_items.map((item, index) => (
                    <div
                      key={index}
                      className="rounded-md border border-warning/20 bg-warning/5 p-3"
                    >
                      <div className="flex items-start gap-3">
                        <AlertCircle className="mt-0.5 size-5 shrink-0 text-warning" />
                        <div className="flex-1">
                          <p className="text-sm font-medium">
                            {item.file_path}
                          </p>
                          <p className="text-muted-foreground mt-0.5 text-xs">
                            {item.reason}
                          </p>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </ScrollArea>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button onClick={onClose}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
