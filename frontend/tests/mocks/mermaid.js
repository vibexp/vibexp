// Stub for the ESM-only `mermaid` package (kept from the jest era).
const mermaid = {
  initialize: vi.fn(),
  render: vi.fn().mockResolvedValue({
    svg: '<svg>mocked mermaid diagram</svg>',
  }),
}

export default mermaid
