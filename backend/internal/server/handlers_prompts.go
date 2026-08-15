package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	promptsgen "github.com/vibexp/vibexp/internal/server/gen/prompts"
	"github.com/vibexp/vibexp/internal/services"
)

// promptsMsgInternalError is the opaque message the strict read handlers return
// for anything unexpected; details go to the log, never to the client.
const promptsMsgInternalError = "Internal server error"

// promptMsgListSuccess is the `message` of the list envelope. It is part of the
// response body, so it is a wire constant, not a log string.
const promptMsgListSuccess = "Prompts retrieved successfully"

// promptMsgListFailed is the 500 detail the list operation has always returned.
const promptMsgListFailed = "Failed to list prompts"

// promptsStrictServer implements the generated Prompts read strict server
// (issue #777, epic #122) — the last of the four read-domain conversions.
//
// Only listPrompts and getPrompt are generated — see oapi-codegen-prompts.yaml
// for why the package is scoped by operation id rather than by tag. The other
// ten Prompts operations keep their chi handlers in prompt_handlers.go and are
// registered beside these.
type promptsStrictServer struct {
	s *Server
}

var _ promptsgen.StrictServerInterface = (*promptsStrictServer)(nil)

// ListPrompts returns a page of the team's prompts, wrapped in the legacy
// {status, message, data} envelope this endpoint has always used.
func (ps *promptsStrictServer) ListPrompts(
	ctx context.Context, request promptsgen.ListPromptsRequestObject,
) (promptsgen.ListPromptsResponseObject, error) {
	userID, err := promptsUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()

	ps.s.logger.With(
		"service", serverLogServiceName,
		"handler", "ListPrompts",
		"user_id", userID,
		"team_id", teamID,
	).Info("List prompts request received")

	filters, err := promptFiltersFromParams(userID, teamID, request.Params)
	if err != nil {
		return nil, err
	}

	response, err := ps.s.container.PromptService().ListPrompts(userID, filters)
	if err != nil {
		ps.s.logger.With(
			"service", serverLogServiceName,
			"handler", "ListPrompts",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error(promptMsgListFailed)
		return nil, apierrors.NewInternalError(promptMsgListFailed)
	}

	attachPageFreshness(ps.s, ctx, teamID, models.RelationResourceTypePrompt,
		response.Prompts, promptID, setPromptFreshness)

	body, err := toGenPromptListEnvelope(response)
	if err != nil {
		ps.s.logger.With("error", err, "operation", "ListPrompts").Error("Prompts response conversion failed")
		return nil, apierrors.NewInternalError(promptMsgListFailed)
	}
	return promptsgen.ListPrompts200JSONResponse(body), nil
}

// GetPrompt returns one prompt with its typed neighborhood attached.
func (ps *promptsStrictServer) GetPrompt(
	ctx context.Context, request promptsgen.GetPromptRequestObject,
) (promptsgen.GetPromptResponseObject, error) {
	userID, err := promptsUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()

	ps.s.logger.With(
		"service", serverLogServiceName,
		"handler", "GetPrompt",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", request.Slug,
	).Info("Get prompt request received")

	// request.Slug is already decoded: the oapi-codegen runtime PathUnescapes
	// path parameters itself (BindStyledParameterOptions.ValueIsUnescaped
	// defaults to false). The chi handler this replaces did not decode at all,
	// and chi hands back an already-decoded segment for a canonical path, so
	// nothing more is needed here.
	prompt, err := ps.s.container.PromptService().GetPromptBySlug(userID, teamID, request.Slug)
	if err != nil {
		return nil, ps.getPromptError(userID, request.Slug, err)
	}

	// Records the access event, via the recordResourceAccess middleware wrapping
	// this mount. The middleware is a no-op unless a handler sets the id, which
	// is why only the detail path produces an event.
	contextkeys.SetAccessedResourceID(ctx, prompt.ID)

	prompt.Related = ps.s.relatedForResource(
		ctx, userID, teamID, models.RelationResourceTypePrompt, prompt.ID,
	)
	prompt.Similar = ps.s.similarForResource(ctx, teamID, models.RelationResourceTypePrompt, prompt.ID)
	prompt.Freshness = ps.s.freshnessForResource(ctx, teamID, models.RelationResourceTypePrompt, prompt.ID)

	body, err := toGenPrompt(prompt)
	if err != nil {
		ps.s.logger.With("error", err, "operation", "GetPrompt").Error("Prompts response conversion failed")
		return nil, apierrors.NewInternalError(promptMsgGetFailed)
	}
	return promptsgen.GetPrompt200JSONResponse(body), nil
}

// getPromptError maps a service error onto the API error the detail operation
// has always returned: a 404 for the sentinel, an opaque 500 for anything else.
// The sentinel check is `errors.Is`, not a string fragment, so it cannot leak
// onto the list operation the way the artifacts/blueprints one could.
func (ps *promptsStrictServer) getPromptError(userID, slug string, err error) error {
	ps.s.logger.With(
		"service", serverLogServiceName,
		"handler", "GetPrompt",
		"user_id", userID,
		"prompt_slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error(promptMsgGetFailed)

	if errors.Is(err, repositories.ErrPromptNotFound) {
		return apierrors.NewResourceNotFoundError("prompt", promptMsgNotFound)
	}
	return apierrors.NewInternalError(promptMsgGetFailed)
}

// promptsUserID reads the authenticated user from the context. The auth
// middleware always sets it, so a miss is a wiring bug rather than a client
// error -- reported as an opaque 500 instead of panicking the request (the chi
// handler this replaces type-asserted, and would have panicked).
func promptsUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		return "", apierrors.NewInternalError(promptsMsgInternalError)
	}
	return userID, nil
}

// promptFiltersFromParams maps the generated query parameters onto the service
// filters, mirroring what parsePromptFilters did from the raw request.
//
// The two hand-rolled value checks are KEPT: oapi-codegen binds an enum as a
// string-typed named type without validating its value, so dropping them would
// silently widen the API — an ignored `freshness` returns the full list and an
// unchecked `sort_by` reaches the repository. `status` and `sort_order` were
// never validated here and still are not.
func promptFiltersFromParams(
	userID, teamID string, params promptsgen.ListPromptsParams,
) (services.PromptFilters, error) {
	freshness := optionalEnumValue(params.Freshness)
	if freshness != "" && freshness != services.FreshnessFilterStale {
		return services.PromptFilters{}, apierrors.NewBadRequestError(
			"freshness must be " + services.FreshnessFilterStale)
	}

	sortBy := optionalEnumValue(params.SortBy)
	if sortBy != "" && !allowedPromptSortFields[sortBy] {
		return services.PromptFilters{}, promptsValidationError("invalid sort_by value: " + sortBy)
	}

	filters := services.PromptFilters{
		Freshness: freshness,
		Status:    optionalEnumValue(params.Status),
		Search:    optionalStringValue(params.Search),
		UserID:    userID,
		TeamID:    teamID,
		SortBy:    sortBy,
		SortOrder: strings.ToLower(optionalEnumValue(params.SortOrder)),
		MCPExpose: params.McpExpose,
		IsShared:  params.Shared,
	}

	if labels := optionalStringValue(params.Labels); labels != "" {
		filters.Labels = strings.Split(labels, ",")
	}
	if params.ProjectId != nil {
		projectID := params.ProjectId.String()
		filters.ProjectID = &projectID
	}

	// validatePaginationParams clamps rather than rejects (page 1..10000, limit
	// 1..100, defaults 1 and 10), which the binder cannot do -- it only converts.
	pagination := validatePaginationParams(
		intPtrToQueryString(params.Page), intPtrToQueryString(params.Limit),
	)
	filters.Page = pagination.Page
	filters.Limit = pagination.Limit

	return filters, nil
}

// promptsValidationError reproduces what writeErrorResponse's "validation_error"
// arm produced: a 400 whose code is CodeValidationFailed rather than the
// bad-request default. Keeping the code identical is what keeps the body
// byte-identical for the sort_by rejection.
func promptsValidationError(detail string) error {
	apiErr := apierrors.NewBadRequestError(detail)
	apiErr.Code = apierrors.CodeValidationFailed
	return apiErr
}

// toGenPromptListEnvelope converts the service list response into the generated
// LEGACY ENVELOPE this endpoint has always emitted: {status, message, data}.
// oapi-codegen flattens the schema's allOf into one struct whose `data` is the
// specific PromptListResponse, so the envelope survives by construction — but
// `status` and `message` are plain strings with no default, so their exact
// values are this function's responsibility.
//
// `make(..., 0, ...)` is what guarantees the required `prompts` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim, so the guarantee has to be built here (this schema
// is registered in adHocRequiredArrayAllowlist for exactly that reason).
func toGenPromptListEnvelope(src *models.PromptListResponse) (promptsgen.PromptListEnvelope, error) {
	prompts := make([]promptsgen.Prompt, 0, len(src.Prompts))
	for i := range src.Prompts {
		converted, err := toGenPrompt(&src.Prompts[i])
		if err != nil {
			return promptsgen.PromptListEnvelope{}, err
		}
		prompts = append(prompts, converted)
	}

	return promptsgen.PromptListEnvelope{
		Status:  "success",
		Message: promptMsgListSuccess,
		Data: promptsgen.PromptListResponse{
			Prompts:    prompts,
			TotalCount: src.TotalCount,
			Page:       src.Page,
			PerPage:    src.PerPage,
			TotalPages: src.TotalPages,
		},
	}, nil
}

// toGenPrompt converts one prompt to its generated representation.
//
// Unlike artifacts and blueprints, models.Prompt and the schema agree field for
// field: nothing on the model is undeclared, so nothing drops off the body
// (#800 item 5 does not apply here — the pre-flight check found no gap).
//
// `labels` is the field to be careful with. It is spec-NULLABLE and a nil
// pq.StringArray serializes as `null` today, but the generated field is a
// `*[]string` with omitempty, where a nil POINTER omits the key entirely.
// Taking the address of the (possibly nil) slice reproduces `null` exactly;
// coercing to `[]` would be the wire change acceptance criterion 6 forbids.
// `related` and `similar` carry no omitempty on the model and so were always
// emitted — they are set unconditionally.
func toGenPrompt(src *models.Prompt) (promptsgen.Prompt, error) {
	teamID, err := promptUUID("team_id", src.TeamID)
	if err != nil {
		return promptsgen.Prompt{}, err
	}
	projectID, err := promptUUID("project_id", src.ProjectID)
	if err != nil {
		return promptsgen.Prompt{}, err
	}

	labels := []string(src.Labels)
	out := promptsgen.Prompt{
		Id:          src.ID,
		Name:        src.Name,
		Slug:        src.Slug,
		Description: src.Description,
		Body:        src.Body,
		UserId:      src.UserID,
		TeamId:      teamID,
		ProjectId:   projectID,
		Status:      promptsgen.PromptStatus(src.Status),
		McpExpose:   src.MCPExpose,
		IsShared:    src.IsShared,
		Labels:      &labels,
		CreatedAt:   src.CreatedAt,
		UpdatedAt:   src.UpdatedAt,
		Version:     src.Version,
	}

	if err := attachGenPromptNeighborhood(&out, src); err != nil {
		return promptsgen.Prompt{}, err
	}
	return out, nil
}

// attachGenPromptNeighborhood copies the three `db:"-"` fields across. Split out
// of toGenPrompt to keep it inside golangci's cognitive-complexity budget; the
// three are one concern anyway -- everything the handler attaches after the
// service call (#421, #427, #735).
func attachGenPromptNeighborhood(out *promptsgen.Prompt, src *models.Prompt) error {
	related := make([]promptsgen.RelatedResource, 0, len(src.Related))
	for _, item := range src.Related {
		converted, err := toGenPromptRelatedResource(item)
		if err != nil {
			return err
		}
		related = append(related, converted)
	}
	out.Related = &related

	similar := make([]promptsgen.SimilarResource, 0, len(src.Similar))
	for _, item := range src.Similar {
		converted, err := toGenPromptSimilarResource(item)
		if err != nil {
			return err
		}
		similar = append(similar, converted)
	}
	out.Similar = &similar

	if src.Freshness != nil {
		freshness, err := toGenPromptFreshnessState(src.Freshness)
		if err != nil {
			return err
		}
		out.Freshness = freshness
	}
	return nil
}

// toGenPromptRelatedResource converts one typed relation neighbor (#421).
func toGenPromptRelatedResource(src models.RelatedResource) (promptsgen.RelatedResource, error) {
	relationID, err := promptUUID("relation_id", src.RelationID)
	if err != nil {
		return promptsgen.RelatedResource{}, err
	}
	resourceID, err := promptUUID("resource_id", src.ResourceID)
	if err != nil {
		return promptsgen.RelatedResource{}, err
	}

	var projectID *openapi_types.UUID
	if src.ProjectID != nil {
		parsed, perr := promptUUID("project_id", *src.ProjectID)
		if perr != nil {
			return promptsgen.RelatedResource{}, perr
		}
		projectID = &parsed
	}

	return promptsgen.RelatedResource{
		RelationId:   relationID,
		RelationType: promptsgen.RelatedResourceRelationType(src.RelationType),
		Direction:    promptsgen.RelatedResourceDirection(src.Direction),
		Origin:       promptsgen.RelatedResourceOrigin(src.Origin),
		Status:       promptsgen.RelatedResourceStatus(src.Status),
		ResourceType: src.ResourceType,
		ResourceId:   resourceID,
		Title:        src.Title,
		Slug:         src.Slug,
		ProjectId:    projectID,
		CreatedAt:    src.CreatedAt,
	}, nil
}

// toGenPromptSimilarResource converts one embedding-similarity neighbor (#427).
func toGenPromptSimilarResource(src models.SimilarResource) (promptsgen.SimilarResource, error) {
	id, err := promptUUID("similar.id", src.ID)
	if err != nil {
		return promptsgen.SimilarResource{}, err
	}
	return promptsgen.SimilarResource{
		Id:    id,
		Type:  src.Type,
		Title: src.Title,
		Score: src.Score,
	}, nil
}

// toGenPromptFreshnessState converts the attached freshness state (#735).
func toGenPromptFreshnessState(
	src *models.ResourceFreshnessState,
) (*promptsgen.ResourceFreshnessState, error) {
	ruleIDs := make([]openapi_types.UUID, 0, len(src.MatchedRuleIDs))
	for _, id := range src.MatchedRuleIDs {
		parsed, err := promptUUID("matched_rule_ids", id)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, parsed)
	}
	return &promptsgen.ResourceFreshnessState{
		Status:         promptsgen.ResourceFreshnessStateStatus(src.Status),
		Reason:         promptsgen.ResourceFreshnessStateReason(src.Reason),
		Since:          src.Since,
		MatchedRuleIds: ruleIDs,
	}, nil
}

// promptUUID parses an id the spec types as a UUID, naming the field so a
// malformed value is diagnosable from the log alone. Note the prompt's OWN id
// and user_id are plain strings in the schema and must not be parsed.
func promptUUID(field, value string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("prompt %s %q is not a UUID: %w", field, value, err)
	}
	return parsed, nil
}

// promptsBindErrorHandler turns a parameter-binding failure into a 400 naming
// the offending parameter.
func (s *Server) promptsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *promptsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		if promptsUUIDParams[invalidParam.ParamName] {
			msg = fmt.Sprintf("%s must be a valid UUID", invalidParam.ParamName)
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// promptsUUIDParams are the parameters the spec types as UUIDs, and so the ones
// a bind failure should report as "must be a valid UUID". `slug` is deliberately
// absent: it is a plain string.
var promptsUUIDParams = map[string]bool{"team_id": true, "project_id": true}

// promptsResponseErrorHandler writes the API error a handler returned. Without
// the errors.As arm every handler error would collapse into a 500, including the
// 404 the spec documents.
func (s *Server) promptsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled prompts handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(promptsMsgInternalError))
}

// promptBoolQueryParams are the query parameters the spec types as booleans AND
// documents as "non-boolean values are ignored (no filter applied)".
var promptBoolQueryParams = []string{"mcp_expose", "shared"}

// dropUnparseableBoolQueryValues removes a boolean query parameter whose value
// is not parseable as a bool, before the generated binder sees it.
//
// Prompts is the only converted domain with boolean filters, and the binder
// would 400 on `?mcp_expose=yes` — where parseBoolParam returned nil, meaning
// "no filter", and where the spec's own description promises the value is
// IGNORED. Stripping the parameter here is what keeps the handler, the chi
// behaviour it replaces, and the published description saying the same thing.
func dropUnparseableBoolQueryValues(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, stripped := withoutUnparseableBools(r.URL.Query(), promptBoolQueryParams)
		if !stripped {
			next.ServeHTTP(w, r)
			return
		}
		// Only RawQuery is reassigned on a deep copy of the request; the URL is
		// never rebuilt from request data (SonarCloud S5144, see
		// dropEmptyQueryValues for the full reasoning).
		scoped := r.Clone(r.Context())
		scoped.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, scoped)
	})
}

// withoutUnparseableBools returns the query with every unparseable value of the
// named boolean parameters removed, and whether it removed any.
func withoutUnparseableBools(query url.Values, names []string) (url.Values, bool) {
	stripped := false
	for _, name := range names {
		values, ok := query[name]
		if !ok {
			continue
		}
		kept := make([]string, 0, len(values))
		for _, value := range values {
			if _, err := strconv.ParseBool(value); err == nil {
				kept = append(kept, value)
			}
		}
		if len(kept) == len(values) {
			continue
		}
		stripped = true
		if len(kept) == 0 {
			query.Del(name)
			continue
		}
		query[name] = kept
	}
	return query, stripped
}
