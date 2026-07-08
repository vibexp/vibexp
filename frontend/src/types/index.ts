// Central type export file - all types are organized into domain-specific files
// and re-exported here for backward compatibility

// Authentication types
export * from './auth'

// API Response types
export * from './api'

// Claude Code Hook and Cursor IDE Hook types
export * from './hooks'

// Alert System types
export * from './alert'

// Resource-agnostic content-versioning types
export * from './version'

// Artifact, Memory, and GitHub integration types now live with their services
// (`@/services/{artifactService,memoryService,githubIntegrationService}`), sourced
// from the generated `@vibexp/api-client` schema — import them from there.
