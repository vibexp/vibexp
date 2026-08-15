package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	memoriesgen "github.com/vibexp/vibexp/internal/server/gen/memories"
	"github.com/vibexp/vibexp/internal/services"
)

// memoriesStrictServer implements the generated Memories read strict server
// (issue #779, epic #122).
//
// Only listMemories and getMemory are generated — see oapi-codegen-memories.yaml
// for why the package is scoped by operation id rather than by tag. Every other
// Memories operation keeps its chi handler in memory_handlers.go and is
// registered beside this one in setupMemoriesRoutes.
type memoriesStrictServer struct {
	s *Server
}

var _ memoriesgen.StrictServerInterface = (*memoriesStrictServer)(nil)

// ListMemories returns a page of the team's memories.
func (ms *memoriesStrictServer) ListMemories(
	ctx context.Context, request memoriesgen.ListMemoriesRequestObject,
) (memoriesgen.ListMemoriesResponseObject, error) {
	userID, err := memoriesUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()

	ms.s.logger.With(
		"service", serverLogServiceName,
		"handler", "ListMemories",
		"user_id", userID,
		"team_id", teamID,
	).Info("List memories request received")

	filters, err := memoryFiltersFromParams(teamID, request.Params)
	if err != nil {
		return nil, err
	}

	response, err := ms.s.container.MemoryService().ListMemories(userID, filters)
	if err != nil {
		return nil, ms.memoriesError("ListMemories", err)
	}

	attachPageFreshness(ms.s, ctx, teamID, models.RelationResourceTypeMemory,
		response.Memories, memoryID, setMemoryFreshness)

	body, err := toGenMemoryListResponse(response)
	if err != nil {
		return nil, ms.memoriesError("ListMemories", err)
	}
	return memoriesgen.ListMemories200JSONResponse(body), nil
}

// GetMemory returns one memory with its typed neighborhood attached.
func (ms *memoriesStrictServer) GetMemory(
	ctx context.Context, request memoriesgen.GetMemoryRequestObject,
) (memoriesgen.GetMemoryResponseObject, error) {
	userID, err := memoriesUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()
	memoryIDParam := request.Id.String()

	ms.s.logger.With(
		"service", serverLogServiceName,
		"handler", "GetMemory",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryIDParam,
	).Info("Get memory request received")

	memory, err := ms.s.container.MemoryService().GetMemory(userID, teamID, memoryIDParam)
	if err != nil {
		return nil, ms.memoriesError("GetMemory", err)
	}

	// Records the access event, via the recordResourceAccess middleware wrapping
	// this mount. The middleware is a no-op unless a handler sets the id, which
	// is why only the detail path produces an event.
	contextkeys.SetAccessedResourceID(ctx, memory.ID)

	memory.Related = ms.s.relatedForResource(
		ctx, userID, teamID, models.RelationResourceTypeMemory, memory.ID,
	)
	memory.Similar = ms.s.similarForResource(ctx, teamID, models.RelationResourceTypeMemory, memory.ID)
	memory.Freshness = ms.s.freshnessForResource(ctx, teamID, models.RelationResourceTypeMemory, memory.ID)

	body, err := toGenMemory(memory)
	if err != nil {
		return nil, ms.memoriesError("GetMemory", err)
	}
	return memoriesgen.GetMemory200JSONResponse(body), nil
}

// memoriesUserID reads the authenticated user from the context. The auth
// middleware always sets it, so a miss is a wiring bug rather than a client
// error -- reported as an opaque 500 instead of panicking the request.
func memoriesUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		return "", apierrors.NewInternalError(memoriesMsgInternalError)
	}
	return userID, nil
}

// memoryFiltersFromParams maps the generated query parameters onto the service
// filters.
//
// The binder covers only what the TYPE system expresses: project_id is parsed
// as a UUID and a malformed one is a 400 before this is reached. It does NOT
// validate enum VALUES -- oapi-codegen binds `freshness`, `status`, `sort_by`
// and `sort_order` as string-typed named types and accepts anything -- so the
// value checks the chi handler carried are still required here. Dropping them
// would send `freshness=stail` to the service as an unrecognized filter, which
// returns the FULL list: a silently ignored filter that looks like a legitimate
// answer, which is exactly what the spec's own description says must 400.
//
// Pagination stays clamped rather than rejected, as validatePaginationParams
// does for every other domain.
func memoryFiltersFromParams(
	teamID string, params memoriesgen.ListMemoriesParams,
) (services.MemoryFilters, error) {
	pagination := validatePaginationParams(
		intPtrToQueryString(params.Page), intPtrToQueryString(params.Limit),
	)

	var projectID *string
	if params.ProjectId != nil {
		value := params.ProjectId.String()
		projectID = &value
	}

	status, freshness, sortBy, err := validatedMemoryEnums(params)
	if err != nil {
		return services.MemoryFilters{}, err
	}

	metadataFilter, err := repositories.ParseMetadataFilter(optionalStringValue(params.Metadata))
	if err != nil {
		return services.MemoryFilters{}, apierrors.NewBadRequestError(err.Error())
	}

	var sortOrder string
	if params.SortOrder != nil {
		sortOrder = strings.ToLower(string(*params.SortOrder))
	}

	return services.MemoryFilters{
		Freshness:      freshness,
		TeamID:         teamID,
		ProjectID:      projectID,
		Search:         optionalStringValue(params.Search),
		MetadataFilter: metadataFilter,
		Status:         status,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
		Page:           pagination.Page,
		Limit:          pagination.Limit,
	}, nil
}

// validatedMemoryEnums checks the three query parameters the spec declares as
// enums. oapi-codegen binds them as string-typed named types and accepts ANY
// value, so these are the checks the chi handler carried and the binder does
// not replace -- see memoryFiltersFromParams for why a silently ignored filter
// is worse than a rejected one.
func validatedMemoryEnums(
	params memoriesgen.ListMemoriesParams,
) (status *string, freshness, sortBy string, err error) {
	if params.Status != nil {
		value := string(*params.Status)
		if !isAllowedMemoryStatus(value) {
			return nil, "", "", apierrors.NewBadRequestError(memoryMsgInvalidStatus)
		}
		status = &value
	}

	if params.Freshness != nil {
		freshness = string(*params.Freshness)
		if freshness != services.FreshnessFilterStale {
			return nil, "", "", apierrors.NewBadRequestError(
				"freshness must be " + services.FreshnessFilterStale)
		}
	}

	if params.SortBy != nil {
		sortBy = string(*params.SortBy)
		if !allowedMemorySortFields[sortBy] {
			return nil, "", "", apierrors.NewBadRequestError("invalid sort_by value: " + sortBy)
		}
	}

	return status, freshness, sortBy, nil
}

// optionalStringValue reads an optional string parameter, absent meaning "".
func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// intPtrToQueryString renders an optional integer parameter the way
// validatePaginationParams expects to receive it: absent becomes "", which is
// what makes it apply the default rather than clamp a zero.
func intPtrToQueryString(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

// toGenMemoryListResponse converts the service list response to the generated
// one.
//
// `make(..., 0, ...)` is what guarantees the required `memories` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim, so the guarantee has to be built here (this schema
// is registered in adHocRequiredArrayAllowlist for exactly that reason).
func toGenMemoryListResponse(src *models.MemoryListResponse) (memoriesgen.MemoryListResponse, error) {
	out := make([]memoriesgen.Memory, 0, len(src.Memories))
	for i := range src.Memories {
		converted, err := toGenMemory(&src.Memories[i])
		if err != nil {
			return memoriesgen.MemoryListResponse{}, err
		}
		out = append(out, converted)
	}
	return memoriesgen.MemoryListResponse{
		Memories:   out,
		TotalCount: src.TotalCount,
		Page:       src.Page,
		PerPage:    src.PerPage,
		TotalPages: src.TotalPages,
	}, nil
}

// toGenMemory converts one memory to its generated representation.
//
// related, similar and freshness carry `db:"-"` and are attached by the handler
// after the service call (#421, #735), so they are mapped explicitly here --
// they are the fields a converter most easily drops, and dropping them is
// invisible to the schema because all three are optional.
//
// The memory's own id fields are plain strings in the spec, but the neighborhood
// types declare theirs as UUIDs, so those are parsed. A parse failure is
// returned rather than swallowed: it would mean the database holds something
// that cannot satisfy the schema, and silently omitting the neighbor would hide
// that behind a valid-looking response.
func toGenMemory(src *models.Memory) (memoriesgen.Memory, error) {
	out := memoriesgen.Memory{
		Id:        src.ID,
		UserId:    src.UserID,
		TeamId:    src.TeamID,
		ProjectId: src.ProjectID,
		Text:      src.Text,
		Status:    memoriesgen.MemoryStatus(src.Status),
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.UpdatedAt,
		Version:   src.Version,
	}

	// Always set, never conditionally: models.Memory declared `metadata` without
	// omitempty, so the hand-marshaled body always carried the key — null for a
	// nil map, {} for an empty one. Taking the address of the (possibly nil) map
	// reproduces both. Omitting the key instead would be a wire change of exactly
	// the kind #122 exists to prevent, and the schema cannot catch it because
	// metadata is optional.
	metadata := map[string]interface{}(src.Metadata)
	out.Metadata = &metadata

	if err := attachGenMemoryNeighborhood(&out, src); err != nil {
		return memoriesgen.Memory{}, err
	}
	return out, nil
}

// attachGenMemoryNeighborhood copies the three `db:"-"` fields across. Split out
// of toGenMemory to keep it inside golangci's cognitive-complexity budget; the
// three are one concern anyway -- everything the handler attaches after the
// service call.
func attachGenMemoryNeighborhood(out *memoriesgen.Memory, src *models.Memory) error {
	// related and similar are likewise always present: they were models.JSONArray
	// fields without omitempty, so an empty neighborhood serialized as [] rather
	// than vanishing. make(..., 0, ...) keeps that true for the empty case.
	related := make([]memoriesgen.RelatedResource, 0, len(src.Related))
	for _, item := range src.Related {
		converted, err := toGenMemoryRelatedResource(item)
		if err != nil {
			return err
		}
		related = append(related, converted)
	}
	out.Related = &related

	similar := make([]memoriesgen.SimilarResource, 0, len(src.Similar))
	for _, item := range src.Similar {
		converted, err := toGenMemorySimilarResource(item)
		if err != nil {
			return err
		}
		similar = append(similar, converted)
	}
	out.Similar = &similar

	if src.Freshness != nil {
		freshness, err := toGenMemoryFreshnessState(src.Freshness)
		if err != nil {
			return err
		}
		out.Freshness = freshness
	}

	return nil
}

// toGenMemoryRelatedResource converts one typed relation neighbor (#421).
func toGenMemoryRelatedResource(src models.RelatedResource) (memoriesgen.RelatedResource, error) {
	relationID, err := memoryUUID("relation_id", src.RelationID)
	if err != nil {
		return memoriesgen.RelatedResource{}, err
	}
	resourceID, err := memoryUUID("resource_id", src.ResourceID)
	if err != nil {
		return memoriesgen.RelatedResource{}, err
	}

	var projectID *openapi_types.UUID
	if src.ProjectID != nil {
		parsed, perr := memoryUUID("project_id", *src.ProjectID)
		if perr != nil {
			return memoriesgen.RelatedResource{}, perr
		}
		projectID = &parsed
	}

	return memoriesgen.RelatedResource{
		RelationId:   relationID,
		RelationType: memoriesgen.RelatedResourceRelationType(src.RelationType),
		Direction:    memoriesgen.RelatedResourceDirection(src.Direction),
		Origin:       memoriesgen.RelatedResourceOrigin(src.Origin),
		Status:       memoriesgen.RelatedResourceStatus(src.Status),
		ResourceType: src.ResourceType,
		ResourceId:   resourceID,
		Title:        src.Title,
		Slug:         src.Slug,
		ProjectId:    projectID,
		CreatedAt:    src.CreatedAt,
	}, nil
}

// toGenMemorySimilarResource converts one embedding-similarity neighbor (#421).
func toGenMemorySimilarResource(src models.SimilarResource) (memoriesgen.SimilarResource, error) {
	id, err := memoryUUID("similar.id", src.ID)
	if err != nil {
		return memoriesgen.SimilarResource{}, err
	}
	return memoriesgen.SimilarResource{
		Id:    id,
		Type:  src.Type,
		Title: src.Title,
		Score: src.Score,
	}, nil
}

// toGenMemoryFreshnessState converts the attached freshness state (#735).
//
// matched_rule_ids is required, so it is built with make(..., 0, ...) for the
// same reason the memories array is.
func toGenMemoryFreshnessState(
	src *models.ResourceFreshnessState,
) (*memoriesgen.ResourceFreshnessState, error) {
	ruleIDs := make([]openapi_types.UUID, 0, len(src.MatchedRuleIDs))
	for _, id := range src.MatchedRuleIDs {
		parsed, err := memoryUUID("matched_rule_ids", id)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, parsed)
	}
	return &memoriesgen.ResourceFreshnessState{
		Status:         memoriesgen.ResourceFreshnessStateStatus(src.Status),
		Reason:         memoriesgen.ResourceFreshnessStateReason(src.Reason),
		Since:          src.Since,
		MatchedRuleIds: ruleIDs,
	}, nil
}

// memoryUUID parses an id the spec types as a UUID, naming the field so a
// malformed value is diagnosable from the log alone.
func memoryUUID(field, value string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("memory %s %q is not a UUID: %w", field, value, err)
	}
	return parsed, nil
}

// memoriesError maps service errors onto API errors. Anything unrecognized is
// logged and reported as an opaque 500.
func (ms *memoriesStrictServer) memoriesError(op string, err error) error {
	if errors.Is(err, repositories.ErrMemoryNotFound) {
		return apierrors.NewResourceNotFoundError("memory", memoryMsgNotFound)
	}
	ms.s.logger.With("error", err, "operation", op).Error("Memories request failed")
	return apierrors.NewInternalError(memoriesMsgInternalError)
}

// memoriesBindErrorHandler turns a parameter-binding failure into a 400 naming
// the offending parameter.
func (s *Server) memoriesBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *memoriesgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		if memoriesUUIDParams[invalidParam.ParamName] {
			msg = fmt.Sprintf("%s must be a valid UUID", invalidParam.ParamName)
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// memoriesUUIDParams are the parameters the spec types as UUIDs, so the bind
// error can say what is actually wrong instead of "not in the expected format".
var memoriesUUIDParams = map[string]bool{"team_id": true, "id": true, "project_id": true}

// dropEmptyQueryValues removes query parameters sent with an empty value before
// the generated binder sees them.
//
// The chi parser this replaced guarded every filter on `!= ""`, so `?status=`,
// `?project_id=`, `?page=` and friends meant "no filter". oapi-codegen's binder
// does not: an empty value is still a PRESENT parameter, so it binds "" and the
// request 400s — turning the common "clear the filter" idiom into an error on
// the list endpoint. Stripping them here restores the old meaning for every
// parameter at once, including the ones (project_id, page, limit) whose failure
// happens inside the binder and cannot be fixed in the handler.
func dropEmptyQueryValues(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, stripped := withoutEmptyValues(r.URL.Query())
		if !stripped {
			next.ServeHTTP(w, r)
			return
		}
		// Clone deep-copies the URL, so rewriting the copy's query leaves the
		// caller's request untouched -- and only RawQuery is reassigned, never
		// the *url.URL itself. Rebuilding the URL is what SonarCloud's S5144
		// flags as constructing an address from user-controlled data; nothing
		// here is ever dereferenced as an outbound address (it is the INBOUND
		// request's own query), but not rebuilding it is the better shape
		// regardless.
		scoped := r.Clone(r.Context())
		scoped.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, scoped)
	})
}

// withoutEmptyValues returns the query with every empty value removed, and
// whether it removed any. A key left with no values at all is dropped entirely,
// which is what makes the parameter read as absent rather than as present-but-
// empty.
func withoutEmptyValues(query url.Values) (url.Values, bool) {
	stripped := false
	for key, values := range query {
		kept := make([]string, 0, len(values))
		for _, value := range values {
			if value != "" {
				kept = append(kept, value)
			}
		}
		if len(kept) == len(values) {
			continue
		}
		stripped = true
		if len(kept) == 0 {
			query.Del(key)
			continue
		}
		query[key] = kept
	}
	return query, stripped
}

// memoriesResponseErrorHandler writes the API error a handler returned. Without
// the errors.As arm every handler error would collapse into a 500 -- including
// the 404 the spec documents for a memory the team does not own.
func (s *Server) memoriesResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled memories handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(memoriesMsgInternalError))
}
