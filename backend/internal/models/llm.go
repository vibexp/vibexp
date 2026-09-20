package models

// CompletionMessage is one turn of a completion request. Role is the
// OpenAI-compatible role name ("system", "user" or "assistant"); the wire format
// of every provider type this backend supports uses those names verbatim.
type CompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest is the provider-agnostic shape of ONE non-streaming
// completion. Streaming, tool-calling and structured output are deliberately
// absent (#1069): a consumer that needs them extends this type together with
// every ModelProvider implementation, rather than reaching around the seam.
//
// Temperature is a pointer because 0 is a meaningful value that must be
// distinguishable from "leave it to the provider's default".
type CompletionRequest struct {
	Messages    []CompletionMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
}

// TokenUsage reports what the completion cost. Both fields are zero when the
// provider does not report usage — an OpenAI-compatible server is not required
// to, so a zero here means "unreported", never "free".
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// CompletionResponse is the first choice of a completion, flattened. FinishReason
// is passed through verbatim ("stop", "length", …) so a consumer can tell a
// truncated answer from a complete one.
type CompletionResponse struct {
	Content      string     `json:"content"`
	FinishReason string     `json:"finish_reason"`
	Usage        TokenUsage `json:"usage"`
}
