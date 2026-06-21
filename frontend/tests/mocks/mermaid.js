// Mock for mermaid library in Jest tests
// Mermaid is an ESM-only module that needs to be mocked for Jest
const mermaid = {
  initialize: jest.fn(),
  render: jest.fn().mockResolvedValue({
    svg: '<svg>mocked mermaid diagram</svg>',
  }),
}

module.exports = mermaid
