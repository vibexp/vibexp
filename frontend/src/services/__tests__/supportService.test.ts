import type { SupportRequest, SupportResponse } from '../supportService'

// Mock the generated client; unwrap stays real so service tests exercise the
// same success/error resolution production uses.
const mockGeneratedClient = {
  POST: jest.fn(),
}

jest.mock('../../lib/apiClientGenerated', () => {
  const actual = jest.requireActual<
    typeof import('../../lib/apiClientGenerated')
  >('../../lib/apiClientGenerated')
  return {
    ...actual,
    generatedClient: mockGeneratedClient,
  }
})

import { supportService } from '../supportService'

const okResponse = { ok: true, status: 200, statusText: 'OK' } as Response

const success = <T>(data: T) => Promise.resolve({ data, response: okResponse })

describe('SupportService', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('posts the support request and returns the response', async () => {
    const response: SupportResponse = { message: 'received', success: true }
    mockGeneratedClient.POST.mockReturnValue(success(response))
    const request: SupportRequest = {
      text: 'Something is broken',
      acknowledgement: true,
      additional_info: { source_url: '/prompts' },
    }

    const result = await supportService.submitSupportRequest(request)

    expect(mockGeneratedClient.POST).toHaveBeenCalledWith(
      '/api/v1/support/message',
      { body: request }
    )
    expect(result).toEqual(response)
  })

  it('throws ApiError with backend detail on RFC 9457 error', async () => {
    mockGeneratedClient.POST.mockReturnValue(
      Promise.resolve({
        error: {
          type: 'https://api.vibexp.io/errors/BAD_REQUEST',
          title: 'Bad Request',
          status: 400,
          detail: 'text is required',
          code: 'BAD_REQUEST',
          request_id: 'req-1',
          timestamp: '2024-01-01T10:00:00Z',
        },
        response: { ok: false, status: 400, statusText: 'Bad Request' },
      })
    )

    await expect(
      supportService.submitSupportRequest({
        text: '',
        acknowledgement: false,
      })
    ).rejects.toThrow('text is required')
  })
})
