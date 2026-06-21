import { ToolOverview } from '@/pages/ai-tools/ToolOverview'
import { apiClient } from '@/utils/api'

export function CursorIDEOverview() {
  return (
    <ToolOverview
      title="Cursor IDE"
      description="AI-powered IDE with intelligent code completion and editing."
      sessionsHref="/ai-tools/cursor-ide/sessions"
      setupHref="/ai-tools/cursor-ide/setup"
      fetchStats={() => apiClient.getCursorOverviewStats()}
      fetchActivities={() => apiClient.getCursorRecentActivities()}
    />
  )
}
