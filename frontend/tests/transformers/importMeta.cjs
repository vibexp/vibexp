/**
 * ts-jest AST transformer: rewrite the `import.meta` meta-property to
 * `globalThis["import.meta"]`.
 *
 * Jest runs ts-jest output as CommonJS, where `import.meta` is a syntax error
 * (TS1343 at compile time, "Cannot use 'import.meta' outside a module" at
 * runtime). The Vite app legitimately reads `import.meta.env` in several
 * modules (siteConfig, sentry, analytics, …) and in some test files. Rather
 * than mock each offender individually, rewrite every `import.meta` to read the
 * `import.meta` object that jest.config.js injects via `globals['import.meta']`.
 *
 * Pair this with ts-jest `diagnostics.ignoreCodes: [1343]` so the (now-rewritten)
 * usage doesn't fail type-checking.
 */
const ts = require('typescript')

const version = 1
const name = 'import-meta-to-global'

function factory() {
  return (ctx) => {
    const visit = (node) => {
      if (
        ts.isMetaProperty(node) &&
        node.keywordToken === ts.SyntaxKind.ImportKeyword &&
        node.name.escapedText === 'meta'
      ) {
        // globalThis["import.meta"]
        return ctx.factory.createElementAccessExpression(
          ctx.factory.createIdentifier('globalThis'),
          ctx.factory.createStringLiteral('import.meta')
        )
      }
      return ts.visitEachChild(node, visit, ctx)
    }
    return (sourceFile) => ts.visitNode(sourceFile, visit)
  }
}

module.exports = { version, name, factory }
