import { ToolOverview } from '@/pages/ai-tools/ToolOverview'
import { apiClient } from '@/utils/api'

export function ClaudeCodeOverview() {
  return (
    <ToolOverview
      title="Claude Code"
      description="AI-powered code assistant with advanced context awareness."
      sessionsHref="/ai-tools/claude-code/sessions"
      setupHref="/ai-tools/claude-code/setup"
      fetchStats={() => apiClient.getOverviewStats()}
      fetchActivities={() => apiClient.getRecentActivities()}
    />
  )
}
