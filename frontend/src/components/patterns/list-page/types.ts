export type SortDir = 'asc' | 'desc'

export type ListPageStatus = 'loading' | 'error' | 'empty' | 'ready'

export interface ListPageCount {
  visible: number
  total: number
  noun: string
  nounPlural?: string
}

export interface ListPagePagination {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
}
