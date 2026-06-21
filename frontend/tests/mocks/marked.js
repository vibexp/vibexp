// Mock for marked library to avoid ES module issues in Jest
const marked = {
  parse: jest.fn(text => `<p>${text}</p>`),
  parseInline: jest.fn(text => text),
}

module.exports = {
  marked,
  default: marked,
}
