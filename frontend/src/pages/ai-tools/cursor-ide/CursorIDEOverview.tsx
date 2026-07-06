import { ToolOverview } from '@/pages/ai-tools/ToolOverview'
import { aiToolsService } from '@/services/aiToolsService'

export function CursorIDEOverview() {
  return (
    <ToolOverview
      title="Cursor IDE"
      description="AI-powered IDE with intelligent code completion and editing."
      sessionsHref="/ai-tools/cursor-ide/sessions"
      setupHref="/ai-tools/cursor-ide/setup"
      fetchStats={() => aiToolsService.getCursorIDEOverviewStats()}
      fetchActivities={() => aiToolsService.getCursorIDERecentActivities()}
    />
  )
}
