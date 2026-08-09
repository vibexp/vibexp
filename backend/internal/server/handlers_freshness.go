package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	freshnessgen "github.com/vibexp/vibexp/internal/server/gen/freshness"
	"github.com/vibexp/vibexp/internal/services"
)

const (
	// freshnessMsgInternalError is the opaque message returned for anything
	// unexpected; details go to the log, never to the client.
	freshnessMsgInternalError = "Internal server error"
	// freshnessMsgForbidden is returned when the caller may not change the
	// team's settings.
	freshnessMsgForbidden = "You do not have permission to change this team's settings"
	// freshnessMsgRuleNotFound is returned for a rule id the team does not own.
	freshnessMsgRuleNotFound = "Freshness rule not found"
)

// freshnessStrictServer implements the generated Freshness strict server.
type freshnessStrictServer struct {
	s *Server
}

var _ freshnessgen.StrictServerInterface = (*freshnessStrictServer)(nil)

// ListFreshnessRules returns the team's rules. Any member may read them; the
// tenancy middleware is the gate.
func (fs *freshnessStrictServer) ListFreshnessRules(
	ctx context.Context, request freshnessgen.ListFreshnessRulesRequestObject,
) (freshnessgen.ListFreshnessRulesResponseObject, error) {
	rules, err := fs.s.container.FreshnessService().ListRules(ctx, request.TeamId.String())
	if err != nil {
		return nil, fs.freshnessError("ListFreshnessRules", err)
	}
	body, err := toGenFreshnessRuleListResponse(rules)
	if err != nil {
		return nil, fs.freshnessError("ListFreshnessRules", err)
	}
	return freshnessgen.ListFreshnessRules200JSONResponse(body), nil
}

// CreateFreshnessRule adds a rule to the team.
func (fs *freshnessStrictServer) CreateFreshnessRule(
	ctx context.Context, request freshnessgen.CreateFreshnessRuleRequestObject,
) (freshnessgen.CreateFreshnessRuleResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	rule, err := fs.s.container.FreshnessService().CreateRule(
		ctx, userID, request.TeamId.String(), inputFromGenCreate(request.Body),
	)
	if err != nil {
		return nil, fs.freshnessError("CreateFreshnessRule", err)
	}
	body, err := toGenFreshnessRule(rule)
	if err != nil {
		return nil, fs.freshnessError("CreateFreshnessRule", err)
	}
	return freshnessgen.CreateFreshnessRule201JSONResponse(body), nil
}

// UpdateFreshnessRule replaces a rule in full.
func (fs *freshnessStrictServer) UpdateFreshnessRule(
	ctx context.Context, request freshnessgen.UpdateFreshnessRuleRequestObject,
) (freshnessgen.UpdateFreshnessRuleResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	rule, err := fs.s.container.FreshnessService().UpdateRule(
		ctx, userID, request.TeamId.String(), request.RuleId.String(), inputFromGenUpdate(request.Body),
	)
	if err != nil {
		return nil, fs.freshnessError("UpdateFreshnessRule", err)
	}
	body, err := toGenFreshnessRule(rule)
	if err != nil {
		return nil, fs.freshnessError("UpdateFreshnessRule", err)
	}
	return freshnessgen.UpdateFreshnessRule200JSONResponse(body), nil
}

// DeleteFreshnessRule removes a rule and the freshness state referencing it.
func (fs *freshnessStrictServer) DeleteFreshnessRule(
	ctx context.Context, request freshnessgen.DeleteFreshnessRuleRequestObject,
) (freshnessgen.DeleteFreshnessRuleResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := fs.s.container.FreshnessService().DeleteRule(
		ctx, userID, request.TeamId.String(), request.RuleId.String(),
	); err != nil {
		return nil, fs.freshnessError("DeleteFreshnessRule", err)
	}
	return freshnessgen.DeleteFreshnessRule204Response{}, nil
}

// GetTeamFreshnessSettings returns the team's settings and their provenance.
func (fs *freshnessStrictServer) GetTeamFreshnessSettings(
	ctx context.Context, request freshnessgen.GetTeamFreshnessSettingsRequestObject,
) (freshnessgen.GetTeamFreshnessSettingsResponseObject, error) {
	view, err := fs.s.container.FreshnessService().GetSettings(ctx, request.TeamId.String())
	if err != nil {
		return nil, fs.freshnessError("GetTeamFreshnessSettings", err)
	}
	return freshnessgen.GetTeamFreshnessSettings200JSONResponse(toGenFreshnessSettings(view)), nil
}

// UpdateTeamFreshnessSettings overrides the team's settings.
func (fs *freshnessStrictServer) UpdateTeamFreshnessSettings(
	ctx context.Context, request freshnessgen.UpdateTeamFreshnessSettingsRequestObject,
) (freshnessgen.UpdateTeamFreshnessSettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("request body is required")
	}

	view, err := fs.s.container.FreshnessService().UpdateSettings(
		ctx, userID, request.TeamId.String(), models.FreshnessSettingsValues{
			IntervalSeconds:      int(request.Body.IntervalSeconds),
			ReversibilityEnabled: request.Body.ReversibilityEnabled,
		},
	)
	if err != nil {
		return nil, fs.freshnessError("UpdateTeamFreshnessSettings", err)
	}
	return freshnessgen.UpdateTeamFreshnessSettings200JSONResponse(toGenFreshnessSettings(view)), nil
}

// ResetTeamFreshnessSettings drops the team's stored settings.
func (fs *freshnessStrictServer) ResetTeamFreshnessSettings(
	ctx context.Context, request freshnessgen.ResetTeamFreshnessSettingsRequestObject,
) (freshnessgen.ResetTeamFreshnessSettingsResponseObject, error) {
	userID, err := authedUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := fs.s.container.FreshnessService().ResetSettings(
		ctx, userID, request.TeamId.String(),
	); err != nil {
		return nil, fs.freshnessError("ResetTeamFreshnessSettings", err)
	}
	return freshnessgen.ResetTeamFreshnessSettings204Response{}, nil
}

// inputFromGenCreate maps the create body to the service input, applying the
// documented defaults for the optional dimensions.
func inputFromGenCreate(body *freshnessgen.CreateFreshnessRuleJSONRequestBody) services.FreshnessRuleInput {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	var mediums []string
	if body.Mediums != nil {
		mediums = mediumsToStrings(*body.Mediums)
	}

	return services.FreshnessRuleInput{
		ProjectID:     uuidPtrToStringPtr(body.ProjectId),
		ResourceTypes: resourceTypesToStrings(body.ResourceTypes),
		Mediums:       mediums,
		ThresholdDays: int(body.ThresholdDays),
		Enabled:       enabled,
	}
}

// inputFromGenUpdate maps the update body to the service input. Every field is
// required by the spec and enforced by requireCompleteFreshnessRuleBody, so
// nothing is defaulted here.
func inputFromGenUpdate(body *freshnessgen.UpdateFreshnessRuleJSONRequestBody) services.FreshnessRuleInput {
	return services.FreshnessRuleInput{
		ProjectID:     uuidPtrToStringPtr(body.ProjectId),
		ResourceTypes: resourceTypesToStrings(body.ResourceTypes),
		Mediums:       mediumsToStrings(body.Mediums),
		ThresholdDays: int(body.ThresholdDays),
		Enabled:       body.Enabled,
	}
}

// toGenFreshnessRuleListResponse converts the service rules to the generated
// list response.
//
// `make(..., 0, ...)` is what guarantees the required `rules` array serializes
// as `[]` and never `null`: generated strict-server types cannot use the
// models.JSONArray shim, so the guarantee has to be built here (this schema is
// registered in adHocRequiredArrayAllowlist for exactly that reason).
func toGenFreshnessRuleListResponse(
	rules []*models.FreshnessRule,
) (freshnessgen.FreshnessRuleListResponse, error) {
	out := make([]freshnessgen.FreshnessRule, 0, len(rules))
	for _, rule := range rules {
		converted, err := toGenFreshnessRule(rule)
		if err != nil {
			return freshnessgen.FreshnessRuleListResponse{}, err
		}
		out = append(out, converted)
	}
	return freshnessgen.FreshnessRuleListResponse{Rules: out}, nil
}

// toGenFreshnessRule converts one rule to its generated representation.
func toGenFreshnessRule(rule *models.FreshnessRule) (freshnessgen.FreshnessRule, error) {
	id, err := uuid.Parse(rule.ID)
	if err != nil {
		return freshnessgen.FreshnessRule{}, fmt.Errorf("freshness rule id %q is not a UUID: %w", rule.ID, err)
	}
	teamID, err := uuid.Parse(rule.TeamID)
	if err != nil {
		return freshnessgen.FreshnessRule{}, fmt.Errorf("freshness rule team_id %q is not a UUID: %w", rule.TeamID, err)
	}
	var projectID *openapi_types.UUID
	if rule.ProjectID != nil {
		parsed, perr := uuid.Parse(*rule.ProjectID)
		if perr != nil {
			return freshnessgen.FreshnessRule{}, fmt.Errorf(
				"freshness rule project_id %q is not a UUID: %w", *rule.ProjectID, perr)
		}
		projectID = &parsed
	}

	mediums := make([]freshnessgen.FreshnessRuleMedium, 0, len(rule.Mediums))
	for _, medium := range rule.Mediums {
		mediums = append(mediums, freshnessgen.FreshnessRuleMedium(medium))
	}
	resourceTypes := make([]freshnessgen.FreshnessRuleResourceType, 0, len(rule.ResourceTypes))
	for _, resourceType := range rule.ResourceTypes {
		resourceTypes = append(resourceTypes, freshnessgen.FreshnessRuleResourceType(resourceType))
	}

	return freshnessgen.FreshnessRule{
		Id:            id,
		TeamId:        teamID,
		ProjectId:     projectID,
		ResourceTypes: resourceTypes,
		Mediums:       mediums,
		ThresholdDays: int32Bounded(rule.ThresholdDays),
		Enabled:       rule.Enabled,
		CreatedAt:     rule.CreatedAt,
		UpdatedAt:     rule.UpdatedAt,
	}, nil
}

// toGenFreshnessSettings converts the settings view to its generated form.
func toGenFreshnessSettings(view *models.TeamFreshnessSettingsView) freshnessgen.TeamFreshnessSettings {
	return freshnessgen.TeamFreshnessSettings{
		Source:               freshnessgen.TeamFreshnessSettingsSource(view.Source),
		IntervalSeconds:      int32Bounded(view.Values.IntervalSeconds),
		ReversibilityEnabled: view.Values.ReversibilityEnabled,
		Defaults: freshnessgen.FreshnessSettingsValues{
			IntervalSeconds:      int32Bounded(view.Defaults.IntervalSeconds),
			ReversibilityEnabled: view.Defaults.ReversibilityEnabled,
		},
	}
}

// int32Bounded narrows an int for the generated response fields, which the spec
// types as int32.
//
// Every value that reaches it is already bounded well inside int32 — both
// threshold_days and interval_seconds carry a schema minimum AND maximum that
// the service validates — so the clamp is unreachable in practice. It exists so
// the narrowing is provably non-wrapping rather than merely asserted to be
// (gosec G115), which is worth more than a suppression comment that stops being
// true the day someone widens a bound.
func int32Bounded(value int) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}

// freshnessError maps service errors onto API errors. Anything unrecognized is
// logged and reported as an opaque 500.
func (fs *freshnessStrictServer) freshnessError(op string, err error) error {
	switch {
	case errors.Is(err, services.ErrPermissionDenied):
		return apierrors.NewForbiddenError(freshnessMsgForbidden)
	case errors.Is(err, repositories.ErrFreshnessRuleNotFound):
		return apierrors.NewResourceNotFoundError("freshness rule", freshnessMsgRuleNotFound)
	case errors.Is(err, services.ErrInvalidFreshnessRule),
		errors.Is(err, services.ErrInvalidFreshnessSettings):
		return apierrors.NewBadRequestError(err.Error())
	default:
		fs.s.logger.With("error", err, "operation", op).Error("Freshness request failed")
		return apierrors.NewInternalError(freshnessMsgInternalError)
	}
}

// freshnessBindErrorHandler turns a parameter-binding failure into a 400 with a
// message naming the offending path parameter.
func (s *Server) freshnessBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *freshnessgen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
		// The UUID params are the ones a caller most often gets wrong, and the
		// generic wording above tells them nothing. Everything else (the
		// paging integers) is better served by the generic message than by a
		// claim about UUIDs, which is what this used to say for every param.
		if freshnessUUIDParams[invalidParam.ParamName] {
			msg = fmt.Sprintf("%s must be a valid UUID", invalidParam.ParamName)
		}
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// freshnessUUIDParams are the domain's uuid-formatted parameters. Before #734
// every bindable parameter here was a UUID, so the bind error handler could
// assume it; the analytics reads added integer query params, and telling
// someone that `page` "must be a valid UUID" sends them looking in the wrong
// place entirely.
var freshnessUUIDParams = map[string]bool{
	"team_id": true,
	"rule_id": true,
}

// freshnessResponseErrorHandler writes the API error a handler returned.
func (s *Server) freshnessResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled freshness handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(freshnessMsgInternalError))
}

// Body fields the two complete-replacement requests require.
var (
	freshnessRuleBodyFields = []string{
		"project_id", "resource_types", "mediums", "threshold_days", "enabled",
	}
	freshnessSettingsBodyFields = []string{"interval_seconds", "reversibility_enabled"}
)

// freshnessBodyProblem reports why a complete-replacement body is unacceptable:
// unknown fields first, then missing ones. Pure, so it is directly testable.
func freshnessBodyProblem(fields map[string]json.RawMessage, required []string) string {
	unknown := make([]string, 0)
	for name := range fields {
		if !slices.Contains(required, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return "unknown field(s): " + strings.Join(unknown, ", ")
	}

	missing := make([]string, 0)
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "missing required field(s): " + strings.Join(missing, ", ")
	}
	return ""
}

// requireCompleteFreshnessBody rejects a PUT body that omits a required field
// or carries an unknown one.
//
// The spec declares `additionalProperties: false` and marks every field
// required, but oapi-codegen enforces NEITHER — a mistyped key would silently
// zero-value that dimension of a complete replacement, turning a typo into a
// policy change. This middleware is what actually enforces the contract, and
// only for PUT: POST /rules is a create with documented optional fields.
func (s *Server) requireCompleteFreshnessBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		required, ok := freshnessRequiredBodyFields(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("request body is required"))
			return
		}
		// Restore the body for the generated binder, which reads it again.
		r.Body = io.NopCloser(bytes.NewReader(raw))

		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			// Malformed JSON is the binder's error to report, with its wording.
			next.ServeHTTP(w, r)
			return
		}
		if problem := freshnessBodyProblem(fields, required); problem != "" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(problem))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// freshnessRequiredBodyFields returns the field set a request must carry in
// full, and whether this request is subject to the check at all.
func freshnessRequiredBodyFields(r *http.Request) ([]string, bool) {
	if r.Method != http.MethodPut {
		return nil, false
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/settings/freshness"):
		return freshnessSettingsBodyFields, true
	case strings.Contains(r.URL.Path, "/freshness/rules/"):
		return freshnessRuleBodyFields, true
	default:
		return nil, false
	}
}

func uuidPtrToStringPtr(value *openapi_types.UUID) *string {
	if value == nil {
		return nil
	}
	s := value.String()
	return &s
}

func resourceTypesToStrings(values []freshnessgen.FreshnessRuleResourceType) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func mediumsToStrings(values []freshnessgen.FreshnessRuleMedium) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
