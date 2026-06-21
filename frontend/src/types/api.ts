// API Response types
export interface ApiResponse<T> {
  status: string
  message: string
  data: T
}

export interface PaginatedData<T> {
  data: T[]
  page: number
  limit: number
  total: number
  total_pages: number
}
