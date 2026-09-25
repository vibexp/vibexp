import { downloadBlob } from '../downloadBlob'

describe('downloadBlob', () => {
  const createObjectURL = vi.fn(() => 'blob:export')
  const revokeObjectURL = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    // jsdom implements neither.
    URL.createObjectURL = createObjectURL
    URL.revokeObjectURL = revokeObjectURL
  })

  it('clicks one temporary link named after the file, then revokes the URL', () => {
    const clicks: HTMLAnchorElement[] = []
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(function (this: HTMLAnchorElement) {
        clicks.push(this)
      })
    const blob = new Blob(['id,email'], { type: 'text/csv' })

    downloadBlob(blob, 'admin-users-20260925.csv')

    expect(createObjectURL).toHaveBeenCalledWith(blob)
    expect(clicks).toHaveLength(1)
    expect(clicks[0].download).toBe('admin-users-20260925.csv')
    expect(clicks[0].href).toBe('blob:export')
    expect(clicks[0].isConnected).toBe(false)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:export')
    click.mockRestore()
  })
})
