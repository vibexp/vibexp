import { Info, Paperclip } from 'lucide-react'

/** Section ids, exported so tests and rail deep-links can address them. */
export const RESOURCE_SECTION_IDS = {
  metadata: 'metadata',
  attachments: 'attachments',
  activity: 'activity',
  comments: 'comments',
  relations: 'relations',
} as const

/**
 * The heading and rail icon of the sections the edit page shares with the
 * reading page (#1180), so the two cannot drift into different names for the
 * same section.
 */
export const RESOURCE_SECTION_CHROME = {
  metadata: { label: 'Metadata', icon: Info },
  attachments: { label: 'Attachments', icon: Paperclip },
} as const
