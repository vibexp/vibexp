// Stub for the ESM-only `marked` package (kept from the jest era — the real
// module resolves under Vitest, but the alias keeps tests deterministic and
// off the full parser).
const marked = {
  parse: vi.fn(text => `<p>${text}</p>`),
  parseInline: vi.fn(text => text),
}

export { marked }
export default marked
