import { getApiBaseUrl } from './environment'

const API_BASE_URL = getApiBaseUrl()

export interface HttpRequestOptions extends Omit<RequestInit, 'headers'> {
  headers?: Record<string, string>
}

class HttpClient {
  async request<T>(
    endpoint: string,
    options: HttpRequestOptions = {}
  ): Promise<T> {
    const url = `${API_BASE_URL}${endpoint}`
    const config: RequestInit = {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      credentials: 'include',
    }

    const response = await fetch(url, config)

    if (!response.ok) {
      if (response.status === 401) {
        // Session expired — redirect to sign-in
        window.location.href = '/sign-in'
        throw new Error('Session expired. Please sign in again.')
      }

      // Try to parse error response, fallback to status text
      let errorMessage = response.statusText
      try {
        const errorData = (await response.json()) as { message?: string }
        if (errorData.message) {
          errorMessage = errorData.message
        }
      } catch {
        // Use statusText if JSON parsing fails
      }
      throw new Error(
        `${errorMessage} (HTTP ${String(response.status)}: ${response.statusText})`
      )
    }

    // External API response - we trust the API to return the expected type
    return response.json() as Promise<T>
  }

  async get<T>(endpoint: string): Promise<T> {
    return this.request<T>(endpoint, { method: 'GET' })
  }

  async post<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    })
  }

  async put<T>(endpoint: string, data?: unknown): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    })
  }

  async delete<T>(endpoint: string): Promise<T> {
    return this.request<T>(endpoint, { method: 'DELETE' })
  }
}

export const httpClient = new HttpClient()
