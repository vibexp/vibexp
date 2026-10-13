import {
  BookOpen,
  FileText,
  HardDrive,
  type LucideIcon,
  Sparkles,
} from 'lucide-react'

import type { DashboardResourceType } from './dashboardLogic'

/** Icon and wording per resource type, shared by every dashboard block. */
export const RESOURCE_TYPE_META: Record<
  DashboardResourceType,
  { icon: LucideIcon; singular: string; plural: string }
> = {
  artifact: { icon: FileText, singular: 'artifact', plural: 'artifacts' },
  blueprint: { icon: BookOpen, singular: 'blueprint', plural: 'blueprints' },
  memory: { icon: HardDrive, singular: 'memory', plural: 'memories' },
  prompt: { icon: Sparkles, singular: 'prompt', plural: 'prompts' },
}
