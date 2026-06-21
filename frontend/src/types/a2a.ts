// A2A (Agent-to-Agent) Streaming Types derived from backend-api
// Reference: backend-api/internal/services/a2a_http_client.go, a2a_stream_processor.go

/**
 * Base part type - all content parts must have a 'kind' field
 */
export interface A2ABasePart {
  kind: string
}

/**
 * Text content part
 */
export interface A2ATextPart extends A2ABasePart {
  kind: 'text'
  text: string
}

/**
 * Code content part (if supported by the A2A agent)
 */
export interface A2ACodePart extends A2ABasePart {
  kind: 'code'
  language: string
  content: string
}

/**
 * Image content part (if supported by the A2A agent)
 */
export interface A2AImagePart extends A2ABasePart {
  kind: 'image'
  source: string
  mimeType?: string
}

/**
 * Union type of all known A2A part types
 * Use this for parts that can be any type
 */
export type A2APart = A2ATextPart | A2ACodePart | A2AImagePart

/**
 * A2A Artifact structure
 * Reference: backend-api/internal/services/a2a_stream_processor.go:handleArtifactUpdate
 */
export interface A2AArtifact {
  artifactId: string
  parts?: A2APart[]
  title?: string
  type?: string
  [key: string]: string | A2APart[] | undefined
}

/**
 * A2A Stream Event Data (artifact-update type)
 * Reference: backend-api/internal/services/a2a_http_client.go:A2AStreamEvent
 */
export interface A2AArtifactUpdateEventData {
  artifactId?: string
  artifact?: A2AArtifact
  append?: boolean
  lastChunk?: boolean
}

/**
 * Generic A2A Stream Event Data
 */
export type A2AStreamEventData =
  | A2AArtifactUpdateEventData
  | Record<string, unknown>

/**
 * A2A Stream Event
 * Reference: backend-api/internal/services/a2a_http_client.go:A2AStreamEvent
 */
export interface A2AStreamEvent {
  type: string
  data: A2AStreamEventData
  timestamp: string
}
