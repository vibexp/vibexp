import {
  Activity,
  Bot,
  FileText,
  Key,
  type LucideIcon,
  User,
} from 'lucide-react'

const ACTIVITY_TYPE_ICONS: Record<string, LucideIcon> = {
  auth_login: User,
  auth_logout: User,
  api_key_created: Key,
  api_key_deleted: Key,
  api_key_used: Key,
  prompt_created: FileText,
  prompt_updated: FileText,
  prompt_deleted: FileText,
  prompt_viewed: FileText,
  prompt_used: FileText,
  artifact_created: FileText,
  artifact_updated: FileText,
  artifact_deleted: FileText,
  artifact_viewed: FileText,
  claude_code_session: Activity,
  claude_code_tool: Activity,
  claude_code_prompt: Activity,
  agent_created: Bot,
  agent_updated: Bot,
  agent_deleted: Bot,
  agent_activated: Bot,
  agent_paused: Bot,
  agent_execution_started: Bot,
  agent_execution_completed: Bot,
  agent_execution_failed: Bot,
  memory_created: FileText,
  memory_updated: FileText,
  memory_deleted: FileText,
  memory_viewed: FileText,
}

const ENTITY_TYPE_ICONS: Record<string, LucideIcon> = {
  api_key: Key,
  prompt: FileText,
  artifact: FileText,
  session: Activity,
  agent: Bot,
  memory: FileText,
  user: User,
}

export function getActivityIcon(
  activityType: string,
  entityType: string
): LucideIcon {
  if (activityType in ACTIVITY_TYPE_ICONS) {
    return ACTIVITY_TYPE_ICONS[activityType]
  }
  if (entityType in ENTITY_TYPE_ICONS) {
    return ENTITY_TYPE_ICONS[entityType]
  }
  return Activity
}
