export interface SupportRequestData {
  text: string
  acknowledgement: boolean
  additional_info?: {
    source_url?: string
    page_id?: string
    subscription_tier?: string
    browser?: string
  }
}

export interface SupportResponse {
  message: string
  success: boolean
}
