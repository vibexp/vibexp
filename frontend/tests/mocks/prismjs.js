// Stub for prismjs in tests (avoids loading the real highlighter).
const prism = {
  highlightAll: vi.fn(),
  highlight: vi.fn(text => text),
  languages: {
    javascript: {},
    typescript: {},
    python: {},
    json: {},
    css: {},
    html: {},
  },
}

export default prism
