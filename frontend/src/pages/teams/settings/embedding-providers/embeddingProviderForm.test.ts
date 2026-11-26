import { schema } from './embeddingProviderForm'

const valid = {
  name: 'P',
  provider_type: 'openai_compatible',
  model: 'm',
  base_url: 'https://api.example.com/v1',
  api_key: '',
  concurrency: 1,
  batch_size: 32,
  request_timeout_seconds: 30,
  query_prefix: '',
  document_prefix: '',
  is_default: false,
  chunk_size: 1000,
  chunk_overlap: 200,
  reprocess: false,
}

// The bounds mirror the server's (#1665): batch_size 1–32, timeout 5–300 s.
describe('embedding provider form schema: batch size and request timeout', () => {
  it.each([
    ['batch_size', 1],
    ['batch_size', 32],
    ['request_timeout_seconds', 5],
    ['request_timeout_seconds', 300],
  ])('accepts %s = %d', (field, value) => {
    expect(schema.safeParse({ ...valid, [field]: value }).success).toBe(true)
  })

  it.each([
    ['batch_size', 0, 'Must be at least 1'],
    ['batch_size', 33, 'Must be at most 32'],
    ['batch_size', 2.5, 'Must be a whole number'],
    ['request_timeout_seconds', 4, 'Must be at least 5'],
    ['request_timeout_seconds', 301, 'Must be at most 300'],
  ])('rejects %s = %d with "%s"', (field, value, message) => {
    const result = schema.safeParse({ ...valid, [field]: value })
    expect(result.success).toBe(false)
    expect(result.error?.issues).toEqual([
      expect.objectContaining({ path: [field], message }),
    ])
  })
})
