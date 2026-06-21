export interface HelpSection {
  id: string
  title: string
  content: string
  order: number
}

export interface HelpContent {
  pageId: string
  title: string
  content: string
  lastUpdated: string
  sections?: HelpSection[]
}

export interface HelpContextValue {
  isOpen: boolean
  currentPageId: string | null
  helpContent: HelpContent | null
  isLoading: boolean
  error: string | null
  openHelp: (pageId: string) => void
  closeHelp: () => void
  setPageId: (pageId: string) => void
}
