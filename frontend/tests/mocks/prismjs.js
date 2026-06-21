module.exports = {
  default: {
    highlightAll: jest.fn(),
    highlight: jest.fn(text => text),
    languages: {
      javascript: {},
      typescript: {},
      python: {},
      json: {},
      css: {},
      html: {},
    },
  },
  __esModule: true,
}
