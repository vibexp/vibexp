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
	artifactsgen "github.com/vibexp/vibexp/internal/server/gen/artifacts"
	"github.com/vibexp/vibexp/internal/services"
)

// artifactsMsgInternalError is the opaque message the strict read handlers
// return for anything unexpected; details go to the log, never to the client.
const artifactsMsgInternalError = "Internal server error"

// artifactsStrictServer implements the generated Artifacts read strict server
// (issue #776, epic #122).
//
// Only listArtifacts, listArtifactsByProject and getArtifact are generated --
// see oapi-codegen-artifacts.yaml for why the package is scoped by operation id
// rather than by tag. The other eleven Artifacts operations keep their chi
// handlers in artifact_handlers.go and are registered beside these.
type artifactsStrictServer struct {
	s *Server
}

var _ artifactsgen.StrictServerInterface = (*artifactsStrictServer)(nil)

// ListArtifacts returns a page of the team's artifacts.
func (as *artifactsStrictServer) ListArtifacts(
	ctx context.Context, request artifactsgen.ListArtifactsRequestObject,
) (artifactsgen.ListArtifactsResponseObject, error) {
	filters, err := artifactFiltersFromQuery(request.TeamId.String(), "", artifactListQuery{
		freshness:    optionalEnumValue(request.Params.Freshness),
		status:       optionalEnumValue(request.Params.Status),
		artifactType: optionalStringValue(request.Params.Type),
		search:       optionalStringValue(request.Params.Search),
		metadata:     optionalStringValue(request.Params.Metadata),
		sortBy:       optionalEnumValue(request.Params.SortBy),
		sortOrder:    optionalEnumValue(request.Params.SortOrder),
		queryProject: request.Params.ProjectId,
		page:         request.Params.Page,
		limit:        request.Params.Limit,
	})
	if err != nil {
		return nil, err
	}

	body, err := as.listArtifacts(ctx, "ListArtifacts", request.TeamId.String(), filters)
	if err != nil {
		return nil, err
	}
	return artifactsgen.ListArtifacts200JSONResponse(body), nil
}

// ListArtifactsByProject is the same listing narrowed to one project, with the
// project taken from the path rather than the query.
func (as *artifactsStrictServer) ListArtifactsByProject(
	ctx context.Context, request artifactsgen.ListArtifactsByProjectRequestObject,
) (artifactsgen.ListArtifactsByProjectResponseObject, error) {
	filters, err := artifactFiltersFromQuery(
		request.TeamId.String(), request.ProjectId.String(), artifactListQuery{
			freshness:    optionalEnumValue(request.Params.Freshness),
			status:       optionalEnumValue(request.Params.Status),
			artifactType: optionalStringValue(request.Params.Type),
			search:       optionalStringValue(request.Params.Search),
			metadata:     optionalStringValue(request.Params.Metadata),
			sortBy:       optionalEnumValue(request.Params.SortBy),
			sortOrder:    optionalEnumValue(request.Params.SortOrder),
			page:         request.Params.Page,
			limit:        request.Params.Limit,
		})
	if err != nil {
		return nil, err
	}

	body, err := as.listArtifacts(ctx, "ListArtifactsByProject", request.TeamId.String(), filters)
	if err != nil {
		return nil, err
	}
	return artifactsgen.ListArtifactsByProject200JSONResponse(body), nil
}

// listArtifacts is the shared body of the two list operations: they differ only
// in where the project id comes from and which response type wraps the result.
func (as *artifactsStrictServer) listArtifacts(
	ctx context.Context, op, teamID string, filters services.ArtifactFilters,
) (artifactsgen.ArtifactListResponse, error) {
	userID, err := artifactsUserID(ctx)
	if err != nil {
		return artifactsgen.ArtifactListResponse{}, err
	}

	as.s.logger.With(
		"service", serverLogServiceName,
		"handler", op,
		"user_id", userID,
		"team_id", teamID,
	).Info("List artifacts request received")

	response, err := as.s.container.ArtifactService().ListArtifacts(userID, filters)
	if err != nil {
		return artifactsgen.ArtifactListResponse{}, as.artifactsError(op, err)
	}

	attachPageFreshness(as.s, ctx, teamID, models.RelationResourceTypeArtifact,
		response.Artifacts, artifactID, setArtifactFreshness)

	body, err := toGenArtifactListResponse(response)
	if err != nil {
		return artifactsgen.ArtifactListResponse{}, as.artifactsError(op, err)
	}
	return body, nil
}

// GetArtifact returns one artifact with its typed neighborhood attached. It is
// keyed on (project_id, slug) rather than an id.
func (as *artifactsStrictServer) GetArtifact(
	ctx context.Context, request artifactsgen.GetArtifactRequestObject,
) (artifactsgen.GetArtifactResponseObject, error) {
	userID, err := artifactsUserID(ctx)
	if err != nil {
		return nil, err
	}
	teamID := request.TeamId.String()
	projectID := request.ProjectId.String()

	as.s.logger.With(
		"service", serverLogServiceName,
		"handler", "GetArtifact",
		"user_id", userID,
		"team_id", teamID,
		"project_id", projectID,
		"slug", request.Slug,
	).Info("Get artifact request received")

	slug, err := artifactSlug(request.Slug)
	if err != nil {
		return nil, err
	}

	artifact, err := as.s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
		userID, teamID, projectID, slug,
	)
	if err != nil {
		return nil, as.artifactsError("GetArtifact", err)
	}

	// Records the access event, via the recordResourceAccess middleware wrapping
	// this mount. The middleware is a no-op unless a handler sets the id, which
	// is why only the detail path produces an event.
	contextkeys.SetAccessedResourceID(ctx, artifact.ID)

	artifact.Related = as.s.relatedForResource(
		ctx, userID, teamID, models.RelationResourceTypeArtifact, artifact.ID,
	)
	artifact.Similar = as.s.similarForResource(ctx, teamID, models.RelationResourceTypeArtifact, artifact.ID)
	artifact.Freshness = as.s.freshnessForResource(
		ctx, teamID, models.RelationResourceTypeArtifact, artifact.ID,
	)

	body, err := toGenArtifact(artifact)
	if err != nil {
		return nil, as.artifactsError("GetArtifact", err)
	}
	return artifactsgen.GetArtifact200JSONResponse(body), nil
}

// artifactsUserID reads the authenticated user from the context. The auth
// middleware always sets it, so a miss is a wiring bug rather than a client
// error -- reported as an opaque 500 instead of panicking the request.
func artifactsUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		return "", apierrors.NewInternalError(artifactsMsgInternalError)
	}
	return userID, nil
}

// artifactFiltersFromQuery maps the generated query parameters onto the
// service filters, mirroring what buildArtifactFilters did from the raw request.
//
// pathProjectID wins over the query parameter, which is how the by-project
// operation narrows the same service call.
//
// Note what is NOT validated here: `status`, `type`, `sort_by` and `sort_order`
// were passed through raw by the chi handler and still are. oapi-codegen binds
// enums as string-typed named types WITHOUT checking their values, so nothing
// rejects an unknown one -- the same silently-ignored filter as before the
// conversion. `freshness` is the exception: it has always been rejected, and
// still is, because an ignored freshness filter returns the full list, which
// reads as a legitimate answer.
// artifactListQuery is the flattened query surface the two list operations
// share. oapi-codegen gives each operation its OWN params struct with its own
// named enum types, so they cannot be converted into one another -- flattening
// at the two call sites keeps the filter mapping in one place instead of two
// near-identical copies that would drift.
type artifactListQuery struct {
	freshness    string
	status       string
	artifactType string
	search       string
	metadata     string
	sortBy       string
	sortOrder    string
	// queryProject is the `project_id` QUERY parameter, which only the
	// team-wide list operation has; the by-project operation takes it from the
	// path instead.
	queryProject *openapi_types.UUID
	page         *int
	limit        *int
}

func artifactFiltersFromQuery(
	teamID, pathProjectID string, query artifactListQuery,
) (services.ArtifactFilters, error) {
	projectID := pathProjectID
	if projectID == "" && query.queryProject != nil {
		projectID = query.queryProject.String()
	}

	if query.freshness != "" && query.freshness != services.FreshnessFilterStale {
		return services.ArtifactFilters{}, apierrors.NewBadRequestError(
			"freshness must be " + services.FreshnessFilterStale)
	}

	metadataFilter, err := repositories.ParseMetadataFilter(query.metadata)
	if err != nil {
		return services.ArtifactFilters{}, apierrors.NewBadRequestError(err.Error())
	}

	pagination := validatePaginationParams(
		intPtrToQueryString(query.page), intPtrToQueryString(query.limit),
	)

	return services.ArtifactFilters{
		Freshness:      query.freshness,
		ProjectID:      projectID,
		TeamID:         teamID,
		Status:         query.status,
		Type:           query.artifactType,
		Search:         query.search,
		SortBy:         query.sortBy,
		SortOrder:      query.sortOrder,
		MetadataFilter: metadataFilter,
		Page:           pagination.Page,
		Limit:          pagination.Limit,
	}, nil
}

// optionalEnumValue reads an optional string-typed enum parameter, absent
// meaning "". The generated enums are distinct named types, so this is generic
// over them rather than reusing optionalStringValue.
func optionalEnumValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

// toGenArtifactListResponse converts the service list response to the generated
// one.
//
// `make(..., 0, ...)` is what guarantees the required `artifacts` array
// serializes as `[]` and never `null`: generated strict-server types cannot use
// the models.JSONArray shim, so the guarantee has to be built here (this schema
// is registered in adHocRequiredArrayAllowlist for exactly that reason).
func toGenArtifactListResponse(src *models.ArtifactListResponse) (artifactsgen.ArtifactListResponse, error) {
	out := make([]artifactsgen.Artifact, 0, len(src.Artifacts))
	for i := range src.Artifacts {
		converted, err := toGenArtifact(&src.Artifacts[i])
		if err != nil {
			return artifactsgen.ArtifactListResponse{}, err
		}
		out = append(out, converted)
	}
	return artifactsgen.ArtifactListResponse{
		Artifacts:  out,
		TotalCount: src.TotalCount,
		Page:       src.Page,
		PerPage:    src.PerPage,
		TotalPages: src.TotalPages,
	}, nil
}

// toGenArtifact converts one artifact to its generated representation.
//
// The optional fields split three ways, and getting this wrong is a silent wire
// change the schema cannot catch (#779):
//   - `description`, `metadata`, `related` and `similar` carry NO omitempty on
//     models.Artifact, so the hand-marshaled body ALWAYS emitted them -- ""
//     null/{} and [] respectively. They are set unconditionally here.
//   - `content` and `freshness` DO carry omitempty, so they are set only when
//     present; emitting them always would add keys that used to be absent, and
//     artifact_freshness_test.go asserts freshness is missing "not even as null".
//
// Note team_id and version are NOT carried: models.Artifact emits them but the
// schema never declared them, so the generated type has no field for them. See
// the PR for #776 and issue #800 -- restoring them would require a spec change,
// which this conversion deliberately does not make.
func toGenArtifact(src *models.Artifact) (artifactsgen.Artifact, error) {
	projectID, err := artifactUUID("project_id", src.ProjectID)
	if err != nil {
		return artifactsgen.Artifact{}, err
	}

	description := src.Description
	metadata := map[string]interface{}(src.Metadata)

	out := artifactsgen.Artifact{
		Id:          src.ID,
		ProjectId:   projectID,
		Slug:        src.Slug,
		UserId:      src.UserID,
		Title:       src.Title,
		Description: &description,
		Type:        src.Type,
		Status:      artifactsgen.ArtifactStatus(src.Status),
		Metadata:    &metadata,
		CreatedAt:   src.CreatedAt,
		UpdatedAt:   src.UpdatedAt,
	}

	if src.Content != "" {
		content := src.Content
		out.Content = &content
	}

	if err := attachGenArtifactNeighborhood(&out, src); err != nil {
		return artifactsgen.Artifact{}, err
	}
	return out, nil
}

// attachGenArtifactNeighborhood copies the three `db:"-"` fields across. Split
// out of toGenArtifact to keep it inside golangci's cognitive-complexity
// budget; the three are one concern anyway -- everything the handler attaches
// after the service call (#421, #735).
func attachGenArtifactNeighborhood(out *artifactsgen.Artifact, src *models.Artifact) error {
	related := make([]artifactsgen.RelatedResource, 0, len(src.Related))
	for _, item := range src.Related {
		converted, err := toGenArtifactRelatedResource(item)
		if err != nil {
			return err
		}
		related = append(related, converted)
	}
	out.Related = &related

	similar := make([]artifactsgen.SimilarResource, 0, len(src.Similar))
	for _, item := range src.Similar {
		converted, err := toGenArtifactSimilarResource(item)
		if err != nil {
			return err
		}
		similar = append(similar, converted)
	}
	out.Similar = &similar

	if src.Freshness != nil {
		freshness, err := toGenArtifactFreshnessState(src.Freshness)
		if err != nil {
			return err
		}
		out.Freshness = freshness
	}
	return nil
}

// toGenArtifactRelatedResource converts one typed relation neighbor (#421).
func toGenArtifactRelatedResource(src models.RelatedResource) (artifactsgen.RelatedResource, error) {
	relationID, err := artifactUUID("relation_id", src.RelationID)
	if err != nil {
		return artifactsgen.RelatedResource{}, err
	}
	resourceID, err := artifactUUID("resource_id", src.ResourceID)
	if err != nil {
		return artifactsgen.RelatedResource{}, err
	}

	var projectID *openapi_types.UUID
	if src.ProjectID != nil {
		parsed, perr := artifactUUID("project_id", *src.ProjectID)
		if perr != nil {
			return artifactsgen.RelatedResource{}, perr
		}
		projectID = &parsed
	}

	return artifactsgen.RelatedResource{
		RelationId:   relationID,
		RelationType: artifactsgen.RelatedResourceRelationType(src.RelationType),
		Direction:    artifactsgen.RelatedResourceDirection(src.Direction),
		Origin:       artifactsgen.RelatedResourceOrigin(src.Origin),
		Status:       artifactsgen.RelatedResourceStatus(src.Status),
		ResourceType: src.ResourceType,
		ResourceId:   resourceID,
		Title:        src.Title,
		Slug:         src.Slug,
		ProjectId:    projectID,
		CreatedAt:    src.CreatedAt,
	}, nil
}

// toGenArtifactSimilarResource converts one embedding-similarity neighbor (#421).
func toGenArtifactSimilarResource(src models.SimilarResource) (artifactsgen.SimilarResource, error) {
	id, err := artifactUUID("similar.id", src.ID)
	if err != nil {
		return artifactsgen.SimilarResource{}, err
	}
	return artifactsgen.SimilarResource{
		Id:    id,
		Type:  src.Type,
		Title: src.Title,
		Score: src.Score,
	}, nil
}

// toGenArtifactFreshnessState converts the attached freshness state (#735).
func toGenArtifactFreshnessState(
	src *models.ResourceFreshnessState,
) (*artifactsgen.ResourceFreshnessState, error) {
	ruleIDs := make([]openapi_types.UUID, 0, len(src.MatchedRuleIDs))
	for _, id := range src.MatchedRuleIDs {
		parsed, err := artifactUUID("matched_rule_ids", id)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, parsed)
	}
	return &artifactsgen.ResourceFreshnessState{
		Status:         artifactsgen.ResourceFreshnessStateStatus(src.Status),
		Reason:         artifactsgen.ResourceFreshnessStateReason(src.Reason),
		Since:          src.Since,
		MatchedRuleIds: ruleIDs,
	}, nil
}

// artifactUUID parses an id the spec types as a UUID, naming the field so a
// malformed value is diagnosable from the log alone.
func artifactUUID(field, value string) (openapi_types.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return openapi_types.UUID{}, fmt.Errorf("artifact %s %q is not a UUID: %w", field, value, err)
	}
	return parsed, nil
}

// artifactsError maps service errors onto API errors. Anything unrecognized is
// logged and reported as an opaque 500.
func (as *artifactsStrictServer) artifactsError(op string, err error) error {
	// The not-found arm matches BOTH ways, because the service layer reports it
	// both ways: handleGetArtifactError detected it by string fragment
	// (artifact_handlers.go), and dropping that in favour of errors.Is alone
	// would silently turn documented 404s into 500s.
	switch {
	case errors.Is(err, repositories.ErrArtifactNotFound),
		strings.Contains(err.Error(), errNotFoundFragment):
		return apierrors.NewResourceNotFoundError("artifact", "Artifact not found")
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError("You do not have permission to access this artifact")
	default:
		as.s.logger.With("error", err, "operation", op).Error("Artifacts request failed")
		return apierrors.NewInternalError(artifactsMsgInternalError)
	}
}

// artifactsBindErrorHandler turns a parameter-binding failure into a 400 naming
// the offending parameter.
func (s *Server) artifactsBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *artifactsgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		if artifactsUUIDParams[invalidParam.ParamName] {
			msg = fmt.Sprintf("%s must be a valid UUID", invalidParam.ParamName)
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// artifactsUUIDParams are the parameters the spec types as UUIDs, so the bind
// error can say what is actually wrong instead of "not in the expected format".
// `slug` is deliberately absent -- it is a plain string.
var artifactsUUIDParams = map[string]bool{"team_id": true, "project_id": true}

// artifactsResponseErrorHandler writes the API error a handler returned.
// Without the errors.As arm every handler error would collapse into a 500,
// including the 404 the spec documents.
func (s *Server) artifactsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled artifacts handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(artifactsMsgInternalError))
}

// artifactSlug decodes the slug path parameter.
//
// chi routes on RawPath whenever the request path contains percent-encoding, so
// chi.URLParam -- and therefore the generated binder, which reads it -- hands
// back the STILL-ENCODED segment (#251/#257). The chi handler this replaced
// called url.PathUnescape explicitly; dropping that would make an exact-match
// lookup miss on every slug containing an encoded character. PathUnescape, not
// QueryUnescape: the two differ on `+`, which QueryUnescape turns into a space.
func artifactSlug(slug string) (string, error) {
	decoded, err := url.PathUnescape(slug)
	if err != nil {
		return "", apierrors.NewBadRequestError("Invalid slug encoding")
	}
	return decoded, nil
}
