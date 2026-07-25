package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
	metadatagen "github.com/vibexp/vibexp/internal/server/gen/metadata"
	"github.com/vibexp/vibexp/internal/services"
)

const metadataMsgInternalError = "Internal server error"

// metadataStrictServer implements metadatagen.StrictServerInterface (epic
// #519): the metadata discovery endpoints at /api/v1/{team_id}/metadata/keys
// and /metadata/values, which tell a client what there is to filter on.
type metadataStrictServer struct {
	s *Server
}

var _ metadatagen.StrictServerInterface = (*metadataStrictServer)(nil)

// GetMetadataKeys handles GET /api/v1/{team_id}/metadata/keys. Any team member
// may read the catalog; membership is enforced by the tenancy middleware and,
// in depth, by the repository's read-access predicate.
func (m *metadataStrictServer) GetMetadataKeys(
	ctx context.Context, request metadatagen.GetMetadataKeysRequestObject,
) (metadatagen.GetMetadataKeysResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	// oapi-codegen generates the Valid() helper for an enum query param but
	// never calls it, so an unknown resource_type would bind as a raw string
	// and fall through to the repository. Reject it here.
	if !request.Params.ResourceType.Valid() {
		return nil, apierrors.NewBadRequestError(metadataUnknownResourceTypeMsg(string(request.Params.ResourceType)))
	}

	query := repositories.MetadataCatalogQuery{
		UserID:       userID,
		TeamID:       request.TeamId.String(),
		ResourceType: repositories.MetadataResourceType(request.Params.ResourceType),
		ProjectID:    metadataProjectID(request.Params.ProjectId),
		Limit:        derefMetadataLimit(request.Params.Limit),
	}

	result, err := m.s.container.MetadataCatalogService().Keys(ctx, query)
	if err != nil {
		return nil, m.metadataError("GetMetadataKeys", err)
	}

	return metadatagen.GetMetadataKeys200JSONResponse{
		// make(...,0) is what guarantees the required `keys` array serializes
		// as [] and never null — a generated type cannot use models.JSONArray.
		Keys:      metadataEntries(result.Entries),
		Truncated: result.Truncated,
	}, nil
}

// GetMetadataValues handles GET /api/v1/{team_id}/metadata/values.
func (m *metadataStrictServer) GetMetadataValues(
	ctx context.Context, request metadatagen.GetMetadataValuesRequestObject,
) (metadatagen.GetMetadataValuesResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if !request.Params.ResourceType.Valid() {
		return nil, apierrors.NewBadRequestError(metadataUnknownResourceTypeMsg(string(request.Params.ResourceType)))
	}

	query := repositories.MetadataCatalogQuery{
		UserID:       userID,
		TeamID:       request.TeamId.String(),
		ResourceType: repositories.MetadataResourceType(request.Params.ResourceType),
		ProjectID:    metadataProjectID(request.Params.ProjectId),
		Key:          request.Params.Key,
		Search:       request.Params.Q,
		Limit:        derefMetadataLimit(request.Params.Limit),
	}

	result, err := m.s.container.MetadataCatalogService().Values(ctx, query)
	if err != nil {
		return nil, m.metadataError("GetMetadataValues", err)
	}

	return metadatagen.GetMetadataValues200JSONResponse{
		// make(...,0): see GetMetadataKeys.
		Values:    metadataEntries(result.Entries),
		Truncated: result.Truncated,
	}, nil
}

// metadataEntries guarantees a non-nil slice, so the required response array
// always serializes as [] rather than null. A generated strict-server type
// cannot use models.JSONArray, so the guarantee has to live here.
func metadataEntries(entries []string) []string {
	if entries == nil {
		return make([]string, 0)
	}
	return entries
}

// metadataProjectID converts the optional UUID query param into the repository's
// optional string.
func metadataProjectID(projectID *openapi_types.UUID) *string {
	if projectID == nil {
		return nil
	}
	value := projectID.String()
	return &value
}

// derefMetadataLimit unwraps the optional limit; zero falls through to the
// repository's clamping default.
func derefMetadataLimit(limit *int) int {
	if limit == nil {
		return 0
	}
	return *limit
}

func metadataUnknownResourceTypeMsg(value string) string {
	return "resource_type must be one of " +
		strings.Join([]string{
			string(repositories.MetadataResourceArtifacts),
			string(repositories.MetadataResourceBlueprints),
			string(repositories.MetadataResourceMemories),
		}, ", ") + "; got " + value
}

// metadataError maps a catalog service error to the RFC 9457 error the strict
// response handler writes. A rejected query is the caller's fault and must
// surface as 400; anything else is ours and is logged before a generic 500.
func (m *metadataStrictServer) metadataError(op string, err error) error {
	if errors.Is(err, services.ErrInvalidMetadataCatalogQuery) {
		return apierrors.NewBadRequestError(err.Error())
	}
	m.s.logger.With("error", err, "operation", op).Error("Metadata catalog request failed")
	return apierrors.NewInternalError(metadataMsgInternalError)
}

// metadataBindErrorHandler translates parameter-binding failures from the
// generated layer into this domain's RFC 9457 400 responses.
func (s *Server) metadataBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *metadatagen.InvalidParamFormatError
	if errors.As(err, &invalidParam) && invalidParam.ParamName == "team_id" {
		msg = "team_id must be a valid UUID"
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// metadataResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) metadataResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Metadata strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(metadataMsgInternalError))
}
