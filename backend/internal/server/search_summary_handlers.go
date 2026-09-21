package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	searchsummarygen "github.com/vibexp/vibexp/internal/server/gen/searchsummary"
	"github.com/vibexp/vibexp/internal/services"
)

// searchSummaryQueryMaxLen mirrors SearchSummaryRequest.query's maxLength. The
// generated binder enforces neither maxLength nor required, so the handler does.
const searchSummaryQueryMaxLen = 1000

const (
	msgSearchSummaryQueryRequired = "query is required"
	msgSearchSummaryQueryTooLong  = "query must be at most 1000 characters"
	msgSearchSummaryInvalidType   = "types must only contain prompts, artifacts, blueprints or memories"
	msgSearchSummaryFailed        = "Failed to generate the summary"
)

// searchSummaryStrictServer implements searchsummarygen.StrictServerInterface:
// POST /api/v1/{team_id}/search/summary (#1073). It shipped strict-server-typed
// because a new endpoint has to (epic #122), while the rest of the Search tag is
// still a hand-written chi handler (search_handlers.go).
type searchSummaryStrictServer struct {
	s *Server
}

var _ searchsummarygen.StrictServerInterface = (*searchSummaryStrictServer)(nil)

// SummarizeSearchResults handles POST /api/v1/{team_id}/search/summary.
func (h *searchSummaryStrictServer) SummarizeSearchResults(
	ctx context.Context, request searchsummarygen.SummarizeSearchResultsRequestObject,
) (searchsummarygen.SummarizeSearchResultsResponseObject, error) {
	teamID := request.TeamId.String()

	req, err := searchSummaryRequestFromGen(request.Body)
	if err != nil {
		return nil, err
	}

	summary, err := h.s.container.SearchSummaryService().Summarize(ctx, teamID, req)
	if err != nil {
		return nil, h.summaryError(teamID, err)
	}

	resp, err := toGenSearchSummaryResponse(summary)
	if err != nil {
		h.s.logger.With(
			"service", serverLogServiceName,
			"handler", "SummarizeSearchResults",
			"team_id", teamID,
			"error", err.Error(),
		).Error("Search summary carried a malformed identifier")
		return nil, apierrors.NewInternalError(msgSearchSummaryFailed)
	}
	return searchsummarygen.SummarizeSearchResults200JSONResponse(resp), nil
}

// searchSummaryRequestFromGen validates the bound body and converts it. The
// generated binder enforces no `required`, `minLength`, `maxLength` or enum on
// a request body, so each is checked here.
func searchSummaryRequestFromGen(body *searchsummarygen.SearchSummaryRequest) (*models.SearchSummaryRequest, error) {
	if body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}
	query := strings.TrimSpace(body.Query)
	if query == "" {
		return nil, apierrors.NewBadRequestError(msgSearchSummaryQueryRequired)
	}
	if utf8.RuneCountInString(body.Query) > searchSummaryQueryMaxLen {
		return nil, apierrors.NewBadRequestError(msgSearchSummaryQueryTooLong)
	}

	req := &models.SearchSummaryRequest{Query: query}
	if body.Types != nil {
		req.Types = make([]string, 0, len(*body.Types))
		for _, t := range *body.Types {
			if !t.Valid() {
				return nil, apierrors.NewBadRequestError(msgSearchSummaryInvalidType)
			}
			req.Types = append(req.Types, string(t))
		}
	}
	// An absent project_id is nil; an all-zero one is a real (if pointless)
	// value that matches no project, so it is passed through rather than
	// treated as "all projects".
	if body.ProjectId != nil {
		req.ProjectID = body.ProjectId.String()
	}
	return req, nil
}

// summaryError maps a service failure onto its classified code. Only the code's
// fixed detail reaches the client; the underlying error — which for a provider
// fault LLMService has already logged with the provider's own response — is
// logged here for everything unclassified.
func (h *searchSummaryStrictServer) summaryError(teamID string, err error) error {
	if code, ok := searchSummaryErrorCode(err); ok {
		return apierrors.NewAISummaryError(code)
	}

	h.s.logger.With(
		"service", serverLogServiceName,
		"handler", "SummarizeSearchResults",
		"team_id", teamID,
		"error", err.Error(),
	).Error(msgSearchSummaryFailed)
	return apierrors.NewInternalError(msgSearchSummaryFailed)
}

// searchSummaryErrorCode classifies a Summarize error. ErrModelRejected and
// ErrContextTooLarge share a code: both mean the provider refused the request,
// and the budgets are what keeps the context small, not something the caller
// can change.
func searchSummaryErrorCode(err error) (string, bool) {
	switch {
	case errors.Is(err, services.ErrAISummaryDisabled):
		return apierrors.CodeAISummaryDisabled, true
	case errors.Is(err, services.ErrNoModelProvider):
		return apierrors.CodeAISummaryNoProvider, true
	case errors.Is(err, services.ErrAISummaryNoResults):
		return apierrors.CodeAISummaryNoResults, true
	case errors.Is(err, services.ErrProviderUnreachable):
		return apierrors.CodeAISummaryProviderUnreachable, true
	case errors.Is(err, services.ErrProviderUnauthorized):
		return apierrors.CodeAISummaryUnauthorized, true
	case errors.Is(err, services.ErrModelRejected), errors.Is(err, services.ErrContextTooLarge):
		return apierrors.CodeAISummaryModelError, true
	case errors.Is(err, services.ErrCompletionTimeout):
		return apierrors.CodeAISummaryTimeout, true
	default:
		return "", false
	}
}

// toGenSearchSummaryResponse converts the domain summary to the wire type.
// Sources is built with make(...,0,n) so it serializes as [] rather than null
// even when empty (issue #125).
func toGenSearchSummaryResponse(summary *models.SearchSummary) (searchsummarygen.SearchSummaryResponse, error) {
	providerID, err := uuid.Parse(summary.ProviderID)
	if err != nil {
		return searchsummarygen.SearchSummaryResponse{}, err
	}

	sources := make([]searchsummarygen.SearchSummarySource, 0, len(summary.Sources))
	for _, src := range summary.Sources {
		id, err := uuid.Parse(src.ID)
		if err != nil {
			return searchsummarygen.SearchSummaryResponse{}, err
		}
		projectID, err := uuid.Parse(src.ProjectID)
		if err != nil {
			return searchsummarygen.SearchSummaryResponse{}, err
		}
		sources = append(sources, searchsummarygen.SearchSummarySource{
			Index:       src.Index,
			Type:        searchsummarygen.SearchSummarySourceType(src.Type),
			Id:          openapi_types.UUID(id),
			Title:       src.Title,
			Slug:        src.Slug,
			ProjectId:   openapi_types.UUID(projectID),
			ProjectName: src.ProjectName,
			UpdatedAt:   src.UpdatedAt,
			Truncated:   src.Truncated,
		})
	}

	resp := searchsummarygen.SearchSummaryResponse{
		Summary:     summary.Summary,
		Sources:     sources,
		Model:       summary.Model,
		ProviderId:  openapi_types.UUID(providerID),
		GeneratedAt: summary.GeneratedAt,
	}
	if summary.Usage != nil {
		resp.Usage = &searchsummarygen.SearchSummaryUsage{
			PromptTokens:     summary.Usage.PromptTokens,
			CompletionTokens: summary.Usage.CompletionTokens,
		}
	}
	return resp, nil
}

// searchSummaryBindErrorHandler translates parameter- and body-binding failures
// from the generated wrapper into RFC 9457 problem details, keeping the
// generated decoder's own wording out of the API.
func (s *Server) searchSummaryBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var invalidParam *searchsummarygen.InvalidParamFormatError
	if errors.As(err, &invalidParam) && invalidParam.ParamName == "team_id" {
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("team_id must be a valid UUID"))
		return
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
}

// searchSummaryResponseErrorHandler writes errors returned by the strict handler
// as RFC 9457 problem details.
func (s *Server) searchSummaryResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Search summary strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
