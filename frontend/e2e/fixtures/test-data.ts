/**
 * Test Data Generators
 *
 * Provides factory functions for generating test data with sensible defaults
 * and support for overrides. Ensures unique data across test runs.
 */

export interface PromptData {
  name: string
  slug: string
  content: string
  tags?: string[]
  isPublic?: boolean
}

export interface ArtifactData {
  title: string
  slug: string
  content: string
  type: 'work_reports' | 'static_contexts' | 'general'
  status?: 'active' | 'expired'
  description?: string
  metadata?: Record<string, string>
}

export interface MemoryData {
  text: string
  metadata?: Record<string, string>
}

/**
 * Generate unique email address for testing
 *
 * @returns Email in format: test_<timestamp>_<random>@example.com
 */
export function generateUniqueEmail(): string {
  const timestamp = Date.now()
  const random = Math.random().toString(36).substring(2, 8)
  return `test_${timestamp}_${random}@example.com`
}

/**
 * Generate unique slug with optional prefix
 *
 * @param prefix - Optional prefix for the slug (default: 'test')
 * @returns Slug in format: <prefix>-<timestamp>-<random>
 */
export function generateUniqueSlug(prefix: string = 'test'): string {
  const timestamp = Date.now()
  const random = Math.random().toString(36).substring(2, 8)
  return `${prefix}-${timestamp}-${random}`
}

/**
 * Generate prompt test data with sensible defaults
 *
 * @param overrides - Optional partial data to override defaults
 * @returns Complete PromptData object
 *
 * @example
 * ```typescript
 * const prompt = generatePromptData({ name: 'My Custom Prompt' })
 * ```
 */
export function generatePromptData(
  overrides?: Partial<PromptData>
): PromptData {
  const defaultSlug = generateUniqueSlug('prompt')

  return {
    name: `Test Prompt ${Date.now()}`,
    slug: defaultSlug,
    content: 'You are a helpful AI assistant. {{task}}',
    tags: ['test', 'automation'],
    isPublic: false,
    ...overrides,
  }
}

/**
 * Generate artifact test data with sensible defaults
 *
 * @param overrides - Optional partial data to override defaults
 * @returns Complete ArtifactData object
 *
 * @example
 * ```typescript
 * const artifact = generateArtifactData({ type: 'work_reports' })
 * ```
 */
export function generateArtifactData(
  overrides?: Partial<ArtifactData>
): ArtifactData {
  const defaultSlug = generateUniqueSlug('artifact')

  return {
    title: `Test Artifact ${Date.now()}`,
    slug: defaultSlug,
    content: '# Test Artifact\n\nThis is test content for artifact testing.',
    type: 'general',
    status: 'active',
    description: 'Test artifact for E2E testing',
    metadata: {
      created_by: 'e2e-test',
      environment: 'test',
    },
    ...overrides,
  }
}

/**
 * Generate memory test data with sensible defaults
 *
 * @param overrides - Optional partial data to override defaults
 * @returns Complete MemoryData object
 *
 * @example
 * ```typescript
 * const memory = generateMemoryData({ text: 'Remember this important fact' })
 * ```
 */
export function generateMemoryData(
  overrides?: Partial<MemoryData>
): MemoryData {
  return {
    text: `Test memory created at ${new Date().toISOString()}`,
    metadata: {
      source: 'e2e-test',
      importance: 'low',
    },
    ...overrides,
  }
}

export interface BlueprintData {
  title: string
  slug: string
  content: string
  description?: string
  type?: 'general' | 'claude-code' | 'claude' | 'cursor' | 'codex'
}

export interface FeedData {
  name: string
  description?: string
}

/**
 * Generate blueprint test data with sensible defaults
 *
 * @param overrides - Optional partial data to override defaults
 * @returns Complete BlueprintData object
 *
 * @example
 * ```typescript
 * const blueprint = generateBlueprintData({ type: 'claude-code' })
 * ```
 */
export function generateBlueprintData(
  overrides?: Partial<BlueprintData>
): BlueprintData {
  return {
    title: `Test Blueprint ${Date.now()}`,
    slug: generateUniqueSlug('blueprint'),
    content: '# Test Blueprint\n\nReusable AI-generated content for E2E tests.',
    description: 'Blueprint created by E2E tests',
    type: 'general',
    ...overrides,
  }
}

/**
 * Generate feed test data with sensible defaults
 *
 * @param overrides - Optional partial data to override defaults
 * @returns Complete FeedData object
 *
 * @example
 * ```typescript
 * const feed = generateFeedData({ name: 'Product Updates' })
 * ```
 */
export function generateFeedData(overrides?: Partial<FeedData>): FeedData {
  return {
    name: `Test Feed ${Date.now()}`,
    description: 'Feed created by E2E tests',
    ...overrides,
  }
}

/**
 * Generate a list of test items with unique data
 *
 * @param count - Number of items to generate
 * @param generator - Generator function to use
 * @param baseOverrides - Base overrides to apply to all items
 * @returns Array of generated items
 *
 * @example
 * ```typescript
 * const prompts = generateList(5, generatePromptData, { isPublic: true })
 * ```
 */
export function generateList<T>(
  count: number,
  generator: (overrides?: Partial<T>) => T,
  baseOverrides?: Partial<T>
): T[] {
  return Array.from({ length: count }, () => generator(baseOverrides))
}
