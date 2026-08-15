package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	blueprintsgen "github.com/vibexp/vibexp/internal/server/gen/blueprints"
	"github.com/vibexp/vibexp/internal/services"
)

// blueprintsMsgInternalError is the opaque message the strict read handlers
// return for anything unexpected; details go to the log, never to the client.
const blueprintsMsgInternalError = "Internal server error"

// blueprintsStrictServer implements the generated Blueprints read strict server
// (issue #778, epic #122).
//
// Only listSpecLibraries, listSpecLibrariesByProject and getBlueprint are
// generated -- see oapi-codegen-blueprints.yaml for why the package is scoped by
// operation id rather than by tag, and for the legacy spelling of those ids. The
// other seven Blueprints operations keep their chi handlers in
// blueprint_handlers.go and are registered beside these.
type blueprintsStrictServer struct {
	s *Server
}

var _ blueprintsgen.StrictServerInterface = (*blueprintsStrictServer)(nil)

// ListSpecLibraries returns a page of the team's blueprints.
func (bs *blueprintsStrictServer) ListSpecLibraries(
	ctx context.Context, request blueprintsgen.ListSpecLibrariesRequestObject,
) (blueprintsgen.ListSpecLibrariesResponseObject, error) {
	query := blueprintListQuery{
		freshness:        optionalEnumValue(request.Params.Freshness),
		status:           optionalEnumValue(request.Params.Status),
		blueprintType:    optionalEnumValue(request.Params.Type),
		subtype:          optionalEnumValue(request.Params.Subtype),
		search:           optionalStringValue(request.Params.Search),
		metadata:         optionalStringValue(request.Params.Metadata),
		sortBy:           optionalEnumValue(request.Params.SortBy),
		sortOrder:        optionalEnumValue(request.Params.SortOrder),
		queryProjectUUID: request.Params.ProjectId,
		page:             request.Params.Page,
		limit:            request.Params.Limit,
	}

	filters, err := blueprintFiltersFromQuery(request.TeamId.String(), "", query)
	if err != nil {
		return nil, err
	}

	body, err := bs.listBlueprints(ctx, "ListSpecLibraries", request.TeamId.String(), filters)
	if err != nil {
		return nil, err
	}
	return blueprintsgen.ListSpecLibraries200JSONResponse(body), nil
}

// ListSpecLibrariesByProject is the same listing narrowed to one project, with
// the project taken from the path rather than the query.
func (bs *blueprintsStrictServer) ListSpecLibrariesByProject(
	ctx context.Context, request blueprintsgen.ListSpecLibrariesByProjectRequestObject,
) (blueprintsgen.ListSpecLibrariesByProjectResponseObject, error) {
	// The spec types this path parameter as a plain string, NOT format: uuid
	// (unlike the artifacts equivalent), so the binder does not reject a
	// malformed one and the handler's own check has to stay -- without it the
	// 400 this endpoint has always returned becomes a 500 from the repository.
	projectID, err := blueprintPathProjectID(request.ProjectId)
	if err != nil {
		return nil, err
	}

	query := blueprintListQuery{
		freshness:     optionalEnumValue(request.Params.Freshness),
		status:        optionalEnumValue(request.Params.Status),
		blueprintType: optionalEnumValue(request.Params.Type),
		subtype:       optionalEnumValue(request.Params.Subtype),
		search:        optionalStringValue(request.Params.Search),
		metadata:      optionalStringValue(request.Params.Metadata),
		sortBy:        optionalEnumValue(request.Params.SortBy),
		sortOrder:     optionalEnumValue(request.Params.SortOrder),
		page:          request.Params.Page,
		limit:         request.Params.Limit,
	}

	filters, err := blueprintFiltersFromQuery(request.TeamId.String(), projectID, query)
	if err != nil {
		return nil, err
	}

	body, err := bs.listBlueprints(ctx, "ListSpecLibrariesByProject", request.TeamId.String(), filters)
	if err != nil {
		return nil, err
	}
	return blueprintsgen.ListSpecLibrariesByProject200JSONResponse(body), nil
}

// listBlueprints is the shared body of the two list operations: they differ only
// in where the project id comes from and which response type wraps the result.
func (bs *blueprintsStrictServer) listBlueprints(
	ctx context.Context, op, teamID string, filters services.BlueprintFilters,
) (blueprintsgen.BlueprintListResponse, error) {
	userID, err := blueprintsUserID(ctx)
	if err != nil {
		return blueprintsgen.BlueprintListResponse{}, err
	}

	bs.s.logger.With(
		"service", serverLogServiceName,
		"handler", op,
		"user_id", userID,
		"team_id", teamID,
	).Info("List blueprints request received")

	response, err := bs.s.container.BlueprintService().ListBlueprints(userID, filters)
	if err != nil {
		return blueprintsgen.BlueprintListResponse{}, bs.blueprintsError(op, err)
	}

	attachPageFreshness(bs.s, ctx, teamID, models.RelationResourceTypeBlueprint,
		response.Blueprints, blueprintID, setBlueprintFreshness)

	body, err := toGenBlueprintListResponse(response)
	if err != nil {
		return blueprintsgen.BlueprintListResponse{}, bs.blueprintsError(op, err)
	}
	return body, nil
}

// GetBlueprint returns one blueprint with its typed neighborhood attached. It is
// keyed on (project_id, slug) and answers with BlueprintDetail, which is
// Blueprint plus raw_content.
func (bs *blueprintsStrictServer) GetBlueprint(
	ctx context.Context, request blueprintsgen.GetBlueprintRequestObject,
) (blueprintsgen.GetBlueprintResponseObject, error) {
	userID, err := blueprintsUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()

	projectID, err := blueprintPathProjectID(request.ProjectId)
	if err != nil {
		return nil, err
	}

	bs.s.logger.With(
		"service", serverLogServiceName,
		"handler", "GetBlueprint",
		"user_id", userID,
		"team_id", teamID,
		"project_id", projectID,
		"slug", request.Slug,
	).Info("Get blueprint request received")

	// request.Slug is already decoded: the oapi-codegen runtime PathUnescapes
	// path parameters itself (BindStyledParameterOptions.ValueIsUnescaped
	// defaults to false), supplying the unescape decodeBlueprintURLParams used
	// to do explicitly. Decoding again would corrupt an encoded percent sign.
	blueprint, err := bs.s.container.BlueprintService().GetBlueprintByProjectIDAndSlugInTeam(
		userID, teamID, projectID, request.Slug,
	)
	if err != nil {
		return nil, bs.blueprintsError("GetBlueprint", err)
	}

	// Records the access event, via the recordResourceAccess middleware wrapping
	// this mount. The middleware is a no-op unless a handler sets the id, which
	// is why only the detail path produces an event.
	contextkeys.SetAccessedResourceID(ctx, blueprint.ID)

	blueprint.Related = bs.s.relatedForResource(
		ctx, userID, teamID, models.RelationResourceTypeBlueprint, blueprint.ID,
	)
	blueprint.Similar = bs.s.similarForResource(ctx, teamID, models.RelationResourceTypeBlueprint, blueprint.ID)
	blueprint.Freshness = bs.s.freshnessForResource(
		ctx, teamID, models.RelationResourceTypeBlueprint, blueprint.ID,
	)

	body, err := toGenBlueprintDetail(blueprint)
	if err != nil {
		return nil, bs.blueprintsError("GetBlueprint", err)
	}
	return blueprintsgen.GetBlueprint200JSONResponse(body), nil
}

// blueprintsUserID reads the authenticated user from the context. The auth
// middleware always sets it, so a miss is a wiring bug rather than a client
// error -- reported as an opaque 500 instead of panicking the request.
func blueprintsUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		return "", apierrors.NewInternalError(blueprintsMsgInternalError)
	}
	return userID, nil
}

// blueprintPathProjectID validates the project_id path parameter, which the
// spec declares as a plain string so the binder lets anything through.
func blueprintPathProjectID(value string) (string, error) {
	if !isValidUUID(value) {
		return "", apierrors.NewBadRequestError(msgInvalidProjectIDFormat)
	}
	return value, nil
}

// blueprintListQuery is the flattened query surface the two list operations
// share. oapi-codegen gives each operation its OWN params struct with its own
// named enum types, so they cannot be converted into one another -- flattening
// at the two call sites keeps the filter mapping in one place.
type blueprintListQuery struct {
	freshness     string
	status        string
	blueprintType string
	subtype       string
	search        string
	metadata      string
	sortBy        string
	sortOrder     string
	// queryProjectUUID is the `project_id` QUERY parameter, which only the
	// team-wide list operation has; the by-project operation takes it from the
	// path instead. The spec DOES type this one as format: uuid.
	queryProjectUUID *openapi_types.UUID
	page             *int
	limit            *int
}

// blueprintFiltersFromQuery maps the generated query parameters onto the service
// filters, mirroring what buildBlueprintFilters did from the raw request.
//
// As on the other converted domains, only `freshness` and `metadata` are
// validated: the chi handler never checked `status`, `type`, `subtype`,
// `sort_by` or `sort_order`, and oapi-codegen does not validate enum VALUES
// either, so those filters stay exactly as permissive as they were. An ignored
// freshness filter, by contrast, returns the full list, which reads as a
// legitimate answer -- so that one is still rejected.
//
// `project_name` is declared on the list operation but has never been read by
// the backend; it stays ignored.
func blueprintFiltersFromQuery(
	teamID, pathProjectID string, query blueprintListQuery,
) (services.BlueprintFilters, error) {
	projectID := pathProjectID
	if projectID == "" && query.queryProjectUUID != nil {
		projectID = query.queryProjectUUID.String()
	}

	if query.freshness != "" && query.freshness != services.FreshnessFilterStale {
		return services.BlueprintFilters{}, apierrors.NewBadRequestError(
			"freshness must be " + services.FreshnessFilterStale)
	}

	metadataFilter, err := repositories.ParseMetadataFilter(query.metadata)
	if err != nil {
		return services.BlueprintFilters{}, apierrors.NewBadRequestError(err.Error())
	}

	pagination := validatePaginationParams(
		intPtrToQueryString(query.page), intPtrToQueryString(query.limit),
	)

	return services.BlueprintFilters{
		Freshness:      query.freshness,
		ProjectID:      projectID,
		TeamID:         teamID,
		Status:         query.status,
		Type:           query.blueprintType,
		Subtype:        query.subtype,
		Search:         query.search,
		SortBy:         query.sortBy,
		SortOrder:      query.sortOrder,
		MetadataFilter: metadataFilter,
		Page:           pagination.Page,
		Limit:          pagination.Limit,
	}, nil
}

// toGenBlueprintListResponse converts the service list response to the generated
// one.
//
// `make(..., 0, ...)` is what guarantees the required `blueprints` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim, so the guarantee has to be built here (this schema
// is registered in adHocRequiredArrayAllowlist for exactly that reason).
func toGenBlueprintListResponse(src *models.BlueprintListResponse) (blueprintsgen.BlueprintListResponse, error) {
	out := make([]blueprintsgen.Blueprint, 0, len(src.Blueprints))
	for i := range src.Blueprints {
		converted, err := toGenBlueprint(&src.Blueprints[i])
		if err != nil {
			return blueprintsgen.BlueprintListResponse{}, err
		}
		out = append(out, converted)
	}
	return blueprintsgen.BlueprintListResponse{
		Blueprints: out,
		TotalCount: src.TotalCount,
		Page:       src.Page,
		PerPage:    src.PerPage,
		TotalPages: src.TotalPages,
	}, nil
}

// toGenBlueprint converts one blueprint to its generated list representation.
//
// The optional fields split two ways, and getting this wrong is a silent wire
// change the schema cannot catch (#779):
//   - `description`, `related` and `similar` carry NO omitempty on
//     models.Blueprint, so the hand-marshaled body ALWAYS emitted them -- "" and
//     [] respectively. They are set unconditionally here.
//   - `metadata`, `subtype`, `content_sha`, `source` and `freshness` DO carry
//     omitempty (metadata unlike its artifacts counterpart), so they are set
//     only when present; emitting them always would add keys that used to be
//     absent.
//
// team_id and version are NOT carried: models.Blueprint emits them but the
// schema never declared them, so the generated type has no field for them. See
// the PR for #778 and issue #800 item 5.
func toGenBlueprint(src *models.Blueprint) (blueprintsgen.Blueprint, error) {
	projectID, err := blueprintUUID("project_id", src.ProjectID)
	if err != nil {
		return blueprintsgen.Blueprint{}, err
	}

	description := src.Description
	out := blueprintsgen.Blueprint{
		Id:          src.ID,
		ProjectId:   projectID,
		Slug:        src.Slug,
		UserId:      src.UserID,
		Title:       src.Title,
		Description: &description,
		Content:     src.Content,
		Path:        src.Path,
		Type:        blueprintsgen.BlueprintType(src.Type),
		Status:      blueprintsgen.BlueprintStatus(src.Status),
		CreatedAt:   src.CreatedAt,
		UpdatedAt:   src.UpdatedAt,
	}

	if len(src.Metadata) > 0 {
		metadata := map[string]interface{}(src.Metadata)
		out.Metadata = &metadata
	}
	if src.Subtype != nil {
		subtype := blueprintsgen.BlueprintSubtype(*src.Subtype)
		out.Subtype = &subtype
	}
	if src.ContentSHA != "" {
		contentSHA := src.ContentSHA
		out.ContentSha = &contentSHA
	}
	if src.Source != nil {
		out.Source = toGenBlueprintSource(src.Source)
	}

	if err := attachGenBlueprintNeighborhood(&out, src); err != nil {
		return blueprintsgen.Blueprint{}, err
	}
	return out, nil
}

// toGenBlueprintDetail converts one blueprint to the DETAIL representation,
// which the spec defines as `allOf: [Blueprint, {raw_content}]` -- so
// oapi-codegen emits a separate struct with its own enum types, and the shared
// fields are copied across from the list form rather than mapped twice.
func toGenBlueprintDetail(src *models.Blueprint) (blueprintsgen.BlueprintDetail, error) {
	base, err := toGenBlueprint(src)
	if err != nil {
		return blueprintsgen.BlueprintDetail{}, err
	}

	out := blueprintsgen.BlueprintDetail{
		Id:          base.Id,
		ProjectId:   base.ProjectId,
		Slug:        base.Slug,
		UserId:      base.UserId,
		Title:       base.Title,
		Description: base.Description,
		Content:     base.Content,
		Path:        base.Path,
		Type:        blueprintsgen.BlueprintDetailType(base.Type),
		Status:      blueprintsgen.BlueprintDetailStatus(base.Status),
		Metadata:    base.Metadata,
		ContentSha:  base.ContentSha,
		Source:      base.Source,
		Related:     base.Related,
		Similar:     base.Similar,
		Freshness:   base.Freshness,
		CreatedAt:   base.CreatedAt,
		UpdatedAt:   base.UpdatedAt,
	}
	if base.Subtype != nil {
		subtype := blueprintsgen.BlueprintDetailSubtype(*base.Subtype)
		out.Subtype = &subtype
	}
	// raw_content is declared on BlueprintDetail only, and carries omitempty on
	// the model: the list query never populates it, and an empty one was absent
	// from the hand-marshaled body.
	if src.RawContent != "" {
		rawContent := src.RawContent
		out.RawContent = &rawContent
	}
	return out, nil
}

// toGenBlueprintSource converts the GitHub-import provenance block.
func toGenBlueprintSource(src *models.BlueprintSource) *blueprintsgen.BlueprintSource {
	out := &blueprintsgen.BlueprintSource{ImportedAt: src.ImportedAt}
	if src.Repo != "" {
		repo := src.Repo
		out.Repo = &repo
	}
	if src.CommitSHA != "" {
		commitSHA := src.CommitSHA
		out.CommitSha = &commitSHA
	}
	if src.BlobSHA != "" {
		blobSHA := src.BlobSHA
		out.BlobSha = &blobSHA
	}
	return out
}

// attachGenBlueprintNeighborhood copies the three `db:"-"` fields across. Split
// out of toGenBlueprint to keep it inside golangci's cognitive-complexity
// budget; the three are one concern anyway -- everything the handler attaches
// after the service call (#421, #735).
func attachGenBlueprintNeighborhood(out *blueprintsgen.Blueprint, src *models.Blueprint) error {
	related := make([]blueprintsgen.RelatedResource, 0, len(src.Related))
	for _, item := range src.Related {
		converted, err := toGenBlueprintRelatedResource(item)
		if err != nil {
			return err
		}
		related = append(related, converted)
	}
	out.Related = &related

	similar := make([]blueprintsgen.SimilarResource, 0, len(src.Similar))
	for _, item := range src.Similar {
		converted, err := toGenBlueprintSimilarResource(item)
		if err != nil {
			return err
		}
		similar = append(similar, converted)
	}
	out.Similar = &similar

	if src.Freshness != nil {
		freshness, err := toGenBlueprintFreshnessState(src.Freshness)
		if err != nil {
			return err
		}
		out.Freshness = freshness
	}
	return nil
}

// toGenBlueprintRelatedResource converts one typed relation neighbor (#421).
func toGenBlueprintRelatedResource(src models.RelatedResource) (blueprintsgen.RelatedResource, error) {
	relationID, err := blueprintUUID("relation_id", src.RelationID)
	if err != nil {
		return blueprintsgen.RelatedResource{}, err
	}
	resourceID, err := blueprintUUID("resource_id", src.ResourceID)
	if err != nil {
		return blueprintsgen.RelatedResource{}, err
	}

	var projectID *openapi_types.UUID
	if src.ProjectID != nil {
		parsed, perr := blueprintUUID("project_id", *src.ProjectID)
		if perr != nil {
			return blueprintsgen.RelatedResource{}, perr
		}
		projectID = &parsed
	}

	return blueprintsgen.RelatedResource{
		RelationId:   relationID,
		RelationType: blueprintsgen.RelatedResourceRelationType(src.RelationType),
		Direction:    blueprintsgen.RelatedResourceDirection(src.Direction),
		Origin:       blueprintsgen.RelatedResourceOrigin(src.Origin),
		Status:       blueprintsgen.RelatedResourceStatus(src.Status),
		ResourceType: src.ResourceType,
		ResourceId:   resourceID,
		Title:        src.Title,
		Slug:         src.Slug,
		ProjectId:    projectID,
		CreatedAt:    src.CreatedAt,
	}, nil
}

// toGenBlueprintSimilarResource converts one embedding-similarity neighbor (#421).
func toGenBlueprintSimilarResource(src models.SimilarResource) (blueprintsgen.SimilarResource, error) {
	id, err := blueprintUUID("similar.id", src.ID)
	if err != nil {
		return blueprintsgen.SimilarResource{}, err
	}
	return blueprintsgen.SimilarResource{
		Id:    id,
		Type:  src.Type,
		Title: src.Title,
		Score: src.Score,
	}, nil
}

// toGenBlueprintFreshnessState converts the attached freshness state (#735).
func toGenBlueprintFreshnessState(
	src *models.ResourceFreshnessState,
) (*blueprintsgen.ResourceFreshnessState, error) {
	ruleIDs := make([]openapi_types.UUID, 0, len(src.MatchedRuleIDs))
	for _, id := range src.MatchedRuleIDs {
		parsed, err := blueprintUUID("matched_rule_ids", id)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, parsed)
	}
	return &blueprintsgen.ResourceFreshnessState{
		Status:         blueprintsgen.ResourceFreshnessStateStatus(src.Status),
		Reason:         blueprintsgen.ResourceFreshnessStateReason(src.Reason),
		Since:          src.Since,
		MatchedRuleIds: ruleIDs,
	}, nil
}

// blueprintUUID parses an id the spec types as a UUID, naming the field so a
// malformed value is diagnosable from the log alone. Note the blueprint's OWN
// id is a plain string in the schema and must not be parsed.
func blueprintUUID(field, value string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("blueprint %s %q is not a UUID: %w", field, value, err)
	}
	return parsed, nil
}

// blueprintsError maps service errors onto API errors. Anything unrecognized is
// logged and reported as an opaque 500.
//
// notFoundIsPossible is false for the list operations: only the detail operation
// documents a 404, and the string-fragment match is broad enough that a list
// failure merely MENTIONING "not found" would otherwise be reported as one.
func (bs *blueprintsStrictServer) blueprintsError(op string, err error) error {
	// The detail handler is the only caller that can produce a not-found, and
	// the chi handler it replaced detected it by string fragment rather than by
	// sentinel -- dropping that would turn documented 404s into 500s. Scoping it
	// to the detail op also keeps a list failure that merely MENTIONS "not
	// found" from surfacing as a status neither list operation documents.
	if op == "GetBlueprint" && strings.Contains(err.Error(), errNotFoundFragment) {
		return apierrors.NewResourceNotFoundError("blueprint", "Blueprint not found")
	}

	bs.s.logger.With("error", err, "operation", op).Error("Blueprints request failed")
	// The 500 body keeps the per-operation wording the chi handlers used, rather
	// than collapsing to a generic message: it costs nothing and it is one fewer
	// wire difference to explain (the equivalent artifacts change is #800 item 1).
	if op == "GetBlueprint" {
		return apierrors.NewInternalError("Failed to get blueprint")
	}
	return apierrors.NewInternalError(blueprintMsgListFailed)
}

// blueprintsBindErrorHandler turns a parameter-binding failure into a 400 naming
// the offending parameter.
func (s *Server) blueprintsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *blueprintsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		if blueprintsUUIDParams[invalidParam.ParamName] {
			msg = fmt.Sprintf("%s must be a valid UUID", invalidParam.ParamName)
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// blueprintsUUIDParams are the parameters the spec types as UUIDs, and so the
// ones a bind failure should be reported as "must be a valid UUID".
//
// project_id is here for the QUERY parameter on the team-wide list, which the
// spec does type as format: uuid. The PATH parameter of the same name is a plain
// string and is rejected by blueprintPathProjectID instead -- it can only reach
// this path via an undecodable percent-escape, where "must be a valid UUID" is
// still true of the value.
var blueprintsUUIDParams = map[string]bool{"team_id": true, "project_id": true}

// blueprintsResponseErrorHandler writes the API error a handler returned.
// Without the errors.As arm every handler error would collapse into a 500,
// including the 404 the spec documents.
func (s *Server) blueprintsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled blueprints handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(blueprintsMsgInternalError))
}
