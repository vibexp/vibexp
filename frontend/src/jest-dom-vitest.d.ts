// Bridges @testing-library/jest-dom's matcher types onto Vitest 5's `expect`.
//
// Vitest 5 inlined its `expect` package: `Assertion` grew a second type
// parameter (`Assertion<R, T>`) and no longer extends the global
// `jest.Matchers`. jest-dom 7.0.1 (the latest release) still augments either
// `jest.Matchers` (its default entry) or a one-parameter `Assertion<T>` (its
// `/vitest` entry), so neither reaches `expect(...)` any more and every
// `toBeInTheDocument()` fails `tsc -b`. The matchers themselves still register
// at runtime; only the types need this. `Matchers<R, T>` is Vitest 5's
// extension point for custom matchers, and an augmentation has to repeat its
// type-parameter list exactly even though jest-dom has no use for `T`.
//
// Delete this file once jest-dom ships Vitest 5 types.
import 'vitest'

import type { TestingLibraryMatchers } from '@testing-library/jest-dom/matchers'

declare module 'vitest' {
  interface Matchers<
    R extends void | Promise<void> = void | Promise<void>,
    T = unknown,
  > extends TestingLibraryMatchers<unknown, R> {}
}
