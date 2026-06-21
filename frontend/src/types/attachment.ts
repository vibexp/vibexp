// Attachment types — generic file attachments for resources (currently artifacts).

export interface Attachment {
  id: string
  team_id: string
  user_id?: string
  owner_type: string
  owner_id: string
  file_name: string
  content_type: string
  size_bytes: number
  created_at: string
}

export interface AttachmentListResponse {
  attachments: Attachment[]
  total_count: number
  total_size_bytes: number
}
