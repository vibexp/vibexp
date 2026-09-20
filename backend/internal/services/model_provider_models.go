package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/vibexp/vibexp/internal/models"
)

// maxModelListResponseBytes caps a GET /models body. Aggregating gateways list
// hundreds of models with per-model metadata, so this is generous, but the
// base_url is caller-supplied and the decode step still needs a ceiling.
const maxModelListResponseBytes = 8 << 20

// errModelListingUnsupported marks a provider that answered but does not
// implement an OpenAI-shaped model listing (a 404, or a body that is not the
// `{"data": [...]}` list). It is reported as supported:false with no failure
// category: the provider is fine, it just cannot enumerate its models.
var errModelListingUnsupported = errors.New("provider does not implement model listing")

// modelListingHTTPError carries a non-2xx from GET /models. Only the status is
// kept — the body never travels, so it cannot reach a client (#464).
type modelListingHTTPError struct {
	StatusCode int
}

func (e *modelListingHTTPError) Error() string {
	return fmt.Sprintf("models endpoint returned status %d", e.StatusCode)
}

type openAIModelListResponse struct {
	Data *[]openAIModelEntry `json:"data"`
}

type openAIModelEntry struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}

// ListModels issues GET {base_url}/models and returns the provider's models,
// sorted by id and unfiltered: a name heuristic that drops embedding/tts models
// would wrongly hide a real one, so choosing is left to the caller.
func (p *OpenAICompatibleModelProvider) ListModels(ctx context.Context) (*models.ProviderModelList, error) {
	result := &models.ProviderModelList{Models: []models.ProviderModel{}}

	entries, err := p.fetchModelList(ctx)
	if err != nil {
		logProviderValidationFailure("model", p.baseURL, err)
		result.Message = classifyModelListingFailure(err)
		return result, nil
	}

	for _, entry := range entries {
		if entry.ID == "" {
			continue
		}
		result.Models = append(result.Models, models.ProviderModel{ID: entry.ID, OwnedBy: entry.OwnedBy})
	}
	sort.SliceStable(result.Models, func(i, j int) bool {
		return result.Models[i].ID < result.Models[j].ID
	})
	result.Supported = true
	return result, nil
}

// fetchModelList performs the request and decodes the OpenAI list envelope.
// Request build + Do live in this one function so gosec's SSRF/bodyclose
// analysers stay satisfied, exactly as in probeModels.
func (p *OpenAICompatibleModelProvider) fetchModelList(ctx context.Context) ([]openAIModelEntry, error) {
	endpoint := p.baseURL + "/models"
	// The destination is caller-supplied on purpose; what bounds it is
	// p.httpClient's SSRF-guarded transport, which refuses reserved ranges at
	// dial time. Do not reinstate a #nosec here (#464).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call models endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &modelListingHTTPError{StatusCode: resp.StatusCode}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxModelListResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read models response: %w", err)
	}

	var decoded openAIModelListResponse
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Data == nil {
		return nil, errModelListingUnsupported
	}
	return *decoded.Data, nil
}

// classifyModelListingFailure maps a failed listing to the fixed category the
// response carries — never the raw error or the URL, whose differences are the
// oracle an internal port scan needs (#464). An empty category means the
// provider answered but does not implement listing, which is not a failure.
func classifyModelListingFailure(err error) string {
	if errors.Is(err, errModelListingUnsupported) {
		return ""
	}
	var httpErr *modelListingHTTPError
	if !errors.As(err, &httpErr) {
		return providerErrConnectionFailed
	}
	switch httpErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return providerErrUnauthorized
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return ""
	default:
		return providerErrConnectionFailed
	}
}
