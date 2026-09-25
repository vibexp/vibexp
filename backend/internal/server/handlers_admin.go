package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
)

// adminMsgInternalError is the generic problem detail for unexpected failures
// in the admin strict handlers.
const adminMsgInternalError = "Internal server error"

// adminStrictServer implements the generated Admin StrictServerInterface. The
// /api/v1/admin surface is guarded by instanceAdminMiddleware, so every request
// reaching these methods is already an authenticated instance admin.
type adminStrictServer struct {
	s *Server
}

var _ admingen.StrictServerInterface = (*adminStrictServer)(nil)

// GetAdminStats returns instance-wide entity counts plus the running app version.
func (a *adminStrictServer) GetAdminStats(
	ctx context.Context, _ admingen.GetAdminStatsRequestObject,
) (admingen.GetAdminStatsResponseObject, error) {
	counts, err := a.s.container.AdminService().GetInstanceCounts(ctx)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName,
			"handler", "GetAdminStats",
			"error", err,
		).Error("Failed to get instance counts")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	return admingen.GetAdminStats200JSONResponse(admingen.AdminStatsResponse{
		Counts: admingen.AdminInstanceCounts{
			Users:     counts.Users,
			Teams:     counts.Teams,
			Prompts:   counts.Prompts,
			Artifacts: counts.Artifacts,
			Memories:  counts.Memories,
		},
		Version: a.appVersion(),
	}), nil
}

// validateAdminSortEnum rejects a sort enum value that the generated binding
// accepted verbatim. oapi-codegen binds an enum query parameter as a plain
// string (BindQueryParameterOptions{Type: "string"}) and never consults the
// enum, so an unknown sort_by/sort_order would silently fall back to the default
// ordering. The generated Valid() helper is the enum definition; calling it here
// is what turns an out-of-enum value into a 400.
func validateAdminSortEnum(name, value string, valid bool) error {
	if !valid {
		return apierrors.NewBadRequestError(fmt.Sprintf("invalid %s value %q", name, value))
	}
	return nil
}

// toAdminUserFilters maps the generated query params onto the repository filter
// struct, validating the sort enums and clamping page/limit.
func toAdminUserFilters(p admingen.ListAdminUsersParams) (repositories.AdminUserFilters, error) {
	page, limit := derefPageLimit(p.Page, p.Limit)
	filters := repositories.AdminUserFilters{
		Search:      p.Search,
		IDPProvider: p.IdpProvider,
		CreatedFrom: p.CreatedFrom,
		CreatedTo:   p.CreatedTo,
		Page:        page,
		Limit:       limit,
	}

	if p.Status != nil {
		if err := validateAdminSortEnum("status", string(*p.Status), p.Status.Valid()); err != nil {
			return repositories.AdminUserFilters{}, err
		}
		status := string(*p.Status)
		filters.Status = &status
	}

	if p.SortBy != nil {
		if err := validateAdminSortEnum("sort_by", string(*p.SortBy), p.SortBy.Valid()); err != nil {
			return repositories.AdminUserFilters{}, err
		}
		filters.SortBy = string(*p.SortBy)
	}
	if p.SortOrder != nil {
		if err := validateAdminSortEnum("sort_order", string(*p.SortOrder), p.SortOrder.Valid()); err != nil {
			return repositories.AdminUserFilters{}, err
		}
		filters.SortOrder = string(*p.SortOrder)
	}

	if err := applyAdminUserAggregateFilters(&filters, p); err != nil {
		return repositories.AdminUserFilters{}, err
	}

	return filters, nil
}

// applyAdminUserAggregateFilters validates the count ranges and the
// last-resource-created range (#1133) and sets them on filters.
func applyAdminUserAggregateFilters(filters *repositories.AdminUserFilters, p admingen.ListAdminUsersParams) error {
	if err := applyAdminCountRanges([]adminCountRangeParam{
		{"team_count", p.TeamCountMin, p.TeamCountMax, &filters.TeamCount},
		{"project_count", p.ProjectCountMin, p.ProjectCountMax, &filters.ProjectCount},
		{"prompt_count", p.PromptCountMin, p.PromptCountMax, &filters.PromptCount},
		{"memory_count", p.MemoryCountMin, p.MemoryCountMax, &filters.MemoryCount},
		{"artifact_count", p.ArtifactCountMin, p.ArtifactCountMax, &filters.ArtifactCount},
		{"blueprint_count", p.BlueprintCountMin, p.BlueprintCountMax, &filters.BlueprintCount},
		{"agent_count", p.AgentCountMin, p.AgentCountMax, &filters.AgentCount},
		{"feed_count", p.FeedCountMin, p.FeedCountMax, &filters.FeedCount},
		{"feed_item_count", p.FeedItemCountMin, p.FeedItemCountMax, &filters.FeedItemCount},
		{"comment_count", p.CommentCountMin, p.CommentCountMax, &filters.CommentCount},
		{"attachment_count", p.AttachmentCountMin, p.AttachmentCountMax, &filters.AttachmentCount},
		{"total_resource_count", p.TotalResourceCountMin, p.TotalResourceCountMax, &filters.TotalResourceCount},
	}); err != nil {
		return err
	}

	if err := validateAdminTimeRange(
		"last_resource_created", p.LastResourceCreatedFrom, p.LastResourceCreatedTo,
	); err != nil {
		return err
	}
	filters.LastResourceCreatedFrom = p.LastResourceCreatedFrom
	filters.LastResourceCreatedTo = p.LastResourceCreatedTo
	return nil
}

// ListAdminUsers returns a paginated, filtered, instance-wide user listing with
// team, project and per-type resource counts.
func (a *adminStrictServer) ListAdminUsers(
	ctx context.Context, request admingen.ListAdminUsersRequestObject,
) (admingen.ListAdminUsersResponseObject, error) {
	filters, err := toAdminUserFilters(request.Params)
	if err != nil {
		return nil, err
	}

	list, err := a.s.container.AdminService().ListUsers(ctx, filters)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminUsers", "error", err,
		).Error("Failed to list admin users")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	genResp, convErr := toGenAdminUserList(list)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminUsers", "error", convErr,
		).Error("Failed to convert admin user list")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.ListAdminUsers200JSONResponse(genResp), nil
}

// GetAdminUser returns one user with their team memberships; an unknown id 404s.
func (a *adminStrictServer) GetAdminUser(
	ctx context.Context, request admingen.GetAdminUserRequestObject,
) (admingen.GetAdminUserResponseObject, error) {
	detail, err := a.s.container.AdminService().GetUserDetail(ctx, request.Id.String())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminUser", "error", err,
		).Error("Failed to get admin user")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("user", "User not found")
	}

	genDetail, convErr := toGenAdminUserDetail(detail)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminUser", "error", convErr,
		).Error("Failed to convert admin user detail")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.GetAdminUser200JSONResponse(genDetail), nil
}

// toGenAdminUserListItem converts a domain user-list row to the generated type,
// parsing the string id into a UUID.
func toGenAdminUserListItem(u models.AdminUserListItem) (admingen.AdminUserListItem, error) {
	id, err := uuid.Parse(u.ID)
	if err != nil {
		return admingen.AdminUserListItem{}, fmt.Errorf("user id %q is not a UUID: %w", u.ID, err)
	}
	return admingen.AdminUserListItem{
		Id:                    id,
		Email:                 openapi_types.Email(u.Email),
		Name:                  u.Name,
		IdpProvider:           u.IDPProvider,
		Status:                admingen.AdminUserListItemStatus(u.Status),
		CreatedAt:             u.CreatedAt,
		TeamCount:             u.TeamCount,
		ProjectCount:          u.ProjectCount,
		ResourceCounts:        toGenAdminResourceCounts(u.ResourceCounts),
		LastResourceCreatedAt: u.LastResourceCreatedAt,
	}, nil
}

// toGenAdminResourceCounts converts per-type authored-resource counts to the
// generated shared AdminResourceCounts schema.
func toGenAdminResourceCounts(c models.AdminResourceCounts) admingen.AdminResourceCounts {
	return admingen.AdminResourceCounts{
		Prompts:     c.Prompts,
		Memories:    c.Memories,
		Artifacts:   c.Artifacts,
		Blueprints:  c.Blueprints,
		Agents:      c.Agents,
		Feeds:       c.Feeds,
		FeedItems:   c.FeedItems,
		Comments:    c.Comments,
		Attachments: c.Attachments,
		Total:       c.Total,
	}
}

// toGenAdminUserList converts a domain user page to the generated response. The
// users slice is always non-nil so the required array serializes as [], not null.
func toGenAdminUserList(l models.AdminUserList) (admingen.AdminUserListResponse, error) {
	users := make([]admingen.AdminUserListItem, 0, len(l.Users))
	for _, u := range l.Users {
		gu, err := toGenAdminUserListItem(u)
		if err != nil {
			return admingen.AdminUserListResponse{}, err
		}
		users = append(users, gu)
	}
	return admingen.AdminUserListResponse{
		Users:      users,
		TotalCount: l.TotalCount,
		Page:       l.Page,
		PerPage:    l.PerPage,
		TotalPages: l.TotalPages,
	}, nil
}

// toGenAdminUserDetail converts a domain user detail to the generated type. The
// memberships slice is always non-nil so the required array serializes as [].
func toGenAdminUserDetail(d *models.AdminUserDetail) (admingen.AdminUserDetail, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return admingen.AdminUserDetail{}, fmt.Errorf("user id %q is not a UUID: %w", d.ID, err)
	}
	memberships := make([]admingen.AdminTeamMembership, 0, len(d.Memberships))
	for _, m := range d.Memberships {
		teamID, parseErr := uuid.Parse(m.TeamID)
		if parseErr != nil {
			return admingen.AdminUserDetail{}, fmt.Errorf("team id %q is not a UUID: %w", m.TeamID, parseErr)
		}
		memberships = append(memberships, admingen.AdminTeamMembership{
			TeamId:   teamID,
			TeamName: m.TeamName,
			Role:     m.Role,
		})
	}
	return admingen.AdminUserDetail{
		Id:          id,
		Email:       openapi_types.Email(d.Email),
		Name:        d.Name,
		IdpProvider: d.IDPProvider,
		Status:      admingen.AdminUserDetailStatus(d.Status),
		CreatedAt:   d.CreatedAt,
		Memberships: memberships,
	}, nil
}

// toAdminTeamFilters maps the generated query params onto the repository filter
// struct, validating the sort enums and clamping page/limit.
func toAdminTeamFilters(p admingen.ListAdminTeamsParams) (repositories.AdminTeamFilters, error) {
	page, limit := derefPageLimit(p.Page, p.Limit)
	filters := repositories.AdminTeamFilters{
		Search:      p.Search,
		IsPersonal:  p.IsPersonal,
		CreatedFrom: p.CreatedFrom,
		CreatedTo:   p.CreatedTo,
		Page:        page,
		Limit:       limit,
	}

	if p.SortBy != nil {
		if err := validateAdminSortEnum("sort_by", string(*p.SortBy), p.SortBy.Valid()); err != nil {
			return repositories.AdminTeamFilters{}, err
		}
		filters.SortBy = string(*p.SortBy)
	}
	if p.SortOrder != nil {
		if err := validateAdminSortEnum("sort_order", string(*p.SortOrder), p.SortOrder.Valid()); err != nil {
			return repositories.AdminTeamFilters{}, err
		}
		filters.SortOrder = string(*p.SortOrder)
	}

	if err := applyAdminTeamAggregateFilters(&filters, p); err != nil {
		return repositories.AdminTeamFilters{}, err
	}

	return filters, nil
}

// applyAdminTeamAggregateFilters validates the count ranges (#1138) and sets
// them, the owner-email match and the configured tri-states on filters.
func applyAdminTeamAggregateFilters(filters *repositories.AdminTeamFilters, p admingen.ListAdminTeamsParams) error {
	if err := applyAdminCountRanges([]adminCountRangeParam{
		{"member_count", p.MemberCountMin, p.MemberCountMax, &filters.MemberCount},
		{"owner_count", p.OwnerCountMin, p.OwnerCountMax, &filters.OwnerCount},
		{"admin_count", p.AdminCountMin, p.AdminCountMax, &filters.AdminCount},
		{"project_count", p.ProjectCountMin, p.ProjectCountMax, &filters.ProjectCount},
		{"prompt_count", p.PromptCountMin, p.PromptCountMax, &filters.PromptCount},
		{"memory_count", p.MemoryCountMin, p.MemoryCountMax, &filters.MemoryCount},
		{"artifact_count", p.ArtifactCountMin, p.ArtifactCountMax, &filters.ArtifactCount},
		{"blueprint_count", p.BlueprintCountMin, p.BlueprintCountMax, &filters.BlueprintCount},
		{"agent_count", p.AgentCountMin, p.AgentCountMax, &filters.AgentCount},
		{"feed_count", p.FeedCountMin, p.FeedCountMax, &filters.FeedCount},
		{"feed_item_count", p.FeedItemCountMin, p.FeedItemCountMax, &filters.FeedItemCount},
		{"comment_count", p.CommentCountMin, p.CommentCountMax, &filters.CommentCount},
		{"attachment_count", p.AttachmentCountMin, p.AttachmentCountMax, &filters.AttachmentCount},
		{"total_resource_count", p.TotalResourceCountMin, p.TotalResourceCountMax, &filters.TotalResourceCount},
	}); err != nil {
		return err
	}

	if p.OwnerEmail != nil {
		if email := strings.TrimSpace(string(*p.OwnerEmail)); email != "" {
			if err := validateAdminEmailParam("owner_email", email); err != nil {
				return err
			}
			filters.OwnerEmail = &email
		}
	}

	filters.EmbeddingConfigured = p.EmbeddingConfigured
	filters.LLMConfigured = p.LlmConfigured
	filters.AISummaryEnabled = p.AiSummaryEnabled
	filters.EmailConfigured = p.EmailConfigured
	filters.GitHubConfigured = p.GithubConfigured
	filters.SearchSettingsCustomized = p.SearchSettingsCustomized
	filters.FreshnessEnabled = p.FreshnessEnabled
	return nil
}

// ListAdminTeams returns a paginated, filtered, instance-wide team listing with
// owner, member/role/project/resource counts and own-configuration state.
func (a *adminStrictServer) ListAdminTeams(
	ctx context.Context, request admingen.ListAdminTeamsRequestObject,
) (admingen.ListAdminTeamsResponseObject, error) {
	filters, err := toAdminTeamFilters(request.Params)
	if err != nil {
		return nil, err
	}

	list, err := a.s.container.AdminService().ListTeams(ctx, filters)
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminTeams", "error", err,
		).Error("Failed to list admin teams")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}

	genResp, convErr := toGenAdminTeamList(list)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "ListAdminTeams", "error", convErr,
		).Error("Failed to convert admin team list")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.ListAdminTeams200JSONResponse(genResp), nil
}

// GetAdminTeam returns one team with owner and member list; an unknown id 404s.
func (a *adminStrictServer) GetAdminTeam(
	ctx context.Context, request admingen.GetAdminTeamRequestObject,
) (admingen.GetAdminTeamResponseObject, error) {
	detail, err := a.s.container.AdminService().GetTeamDetail(ctx, request.Id.String())
	if err != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminTeam", "error", err,
		).Error("Failed to get admin team")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	if detail == nil {
		return nil, apierrors.NewResourceNotFoundError("team", "Team not found")
	}

	genDetail, convErr := toGenAdminTeamDetail(detail)
	if convErr != nil {
		a.s.logger.With(
			"service", serverLogServiceName, "handler", "GetAdminTeam", "error", convErr,
		).Error("Failed to convert admin team detail")
		return nil, apierrors.NewInternalError(adminMsgInternalError)
	}
	return admingen.GetAdminTeam200JSONResponse(genDetail), nil
}

// toGenAdminTeamOwner converts a domain team owner to the generated type.
func toGenAdminTeamOwner(o models.AdminTeamOwner) (admingen.AdminTeamOwner, error) {
	id, err := uuid.Parse(o.ID)
	if err != nil {
		return admingen.AdminTeamOwner{}, fmt.Errorf("team owner id %q is not a UUID: %w", o.ID, err)
	}
	return admingen.AdminTeamOwner{Id: id, Email: openapi_types.Email(o.Email), Name: o.Name}, nil
}

// toGenAdminTeamListItem converts a domain team-list row to the generated type.
func toGenAdminTeamListItem(t models.AdminTeamListItem) (admingen.AdminTeamListItem, error) {
	id, err := uuid.Parse(t.ID)
	if err != nil {
		return admingen.AdminTeamListItem{}, fmt.Errorf("team id %q is not a UUID: %w", t.ID, err)
	}
	owner, err := toGenAdminTeamOwner(t.Owner)
	if err != nil {
		return admingen.AdminTeamListItem{}, err
	}
	return admingen.AdminTeamListItem{
		Id:             id,
		Name:           t.Name,
		Slug:           t.Slug,
		IsPersonal:     t.IsPersonal,
		Owner:          owner,
		MemberCount:    t.MemberCount,
		OwnerCount:     t.OwnerCount,
		AdminCount:     t.AdminCount,
		ProjectCount:   t.ProjectCount,
		ResourceCounts: toGenAdminResourceCounts(t.ResourceCounts),
		Configuration:  toGenAdminTeamConfiguration(t.Configuration),
		CreatedAt:      t.CreatedAt,
	}, nil
}

// toGenAdminTeamConfiguration converts a team's own-configuration flags to the
// generated type.
func toGenAdminTeamConfiguration(c models.AdminTeamConfiguration) admingen.AdminTeamConfiguration {
	return admingen.AdminTeamConfiguration{
		EmbeddingConfigured:      c.EmbeddingConfigured,
		LlmConfigured:            c.LLMConfigured,
		AiSummaryEnabled:         c.AISummaryEnabled,
		EmailConfigured:          c.EmailConfigured,
		GithubConfigured:         c.GitHubConfigured,
		SearchSettingsCustomized: c.SearchSettingsCustomized,
		FreshnessEnabled:         c.FreshnessEnabled,
	}
}

// toGenAdminTeamList converts a domain team page to the generated response. The
// teams slice is always non-nil so the required array serializes as [], not null.
func toGenAdminTeamList(l models.AdminTeamList) (admingen.AdminTeamListResponse, error) {
	teams := make([]admingen.AdminTeamListItem, 0, len(l.Teams))
	for _, t := range l.Teams {
		gt, err := toGenAdminTeamListItem(t)
		if err != nil {
			return admingen.AdminTeamListResponse{}, err
		}
		teams = append(teams, gt)
	}
	return admingen.AdminTeamListResponse{
		Teams:      teams,
		TotalCount: l.TotalCount,
		Page:       l.Page,
		PerPage:    l.PerPage,
		TotalPages: l.TotalPages,
	}, nil
}

// toGenAdminTeamMember converts a domain team member to the generated type.
func toGenAdminTeamMember(m models.AdminTeamMember) (admingen.AdminTeamMember, error) {
	uid, err := uuid.Parse(m.UserID)
	if err != nil {
		return admingen.AdminTeamMember{}, fmt.Errorf("team member user id %q is not a UUID: %w", m.UserID, err)
	}
	return admingen.AdminTeamMember{
		UserId:   uid,
		Email:    openapi_types.Email(m.Email),
		Name:     m.Name,
		Role:     m.Role,
		JoinedAt: m.JoinedAt,
	}, nil
}

// toGenAdminTeamDetail converts a domain team detail to the generated type. The
// members slice is always non-nil so the required array serializes as [].
func toGenAdminTeamDetail(d *models.AdminTeamDetail) (admingen.AdminTeamDetail, error) {
	id, err := uuid.Parse(d.ID)
	if err != nil {
		return admingen.AdminTeamDetail{}, fmt.Errorf("team id %q is not a UUID: %w", d.ID, err)
	}
	owner, err := toGenAdminTeamOwner(d.Owner)
	if err != nil {
		return admingen.AdminTeamDetail{}, err
	}
	members := make([]admingen.AdminTeamMember, 0, len(d.Members))
	for _, m := range d.Members {
		gm, memErr := toGenAdminTeamMember(m)
		if memErr != nil {
			return admingen.AdminTeamDetail{}, memErr
		}
		members = append(members, gm)
	}
	return admingen.AdminTeamDetail{
		Id:         id,
		Name:       d.Name,
		Slug:       d.Slug,
		IsPersonal: d.IsPersonal,
		Owner:      owner,
		CreatedAt:  d.CreatedAt,
		Members:    members,
	}, nil
}

// adminBindErrorHandler translates parameter-binding failures from the generated
// layer into this domain's RFC 9457 400 responses.
func (s *Server) adminBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(err.Error()))
}

// adminResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) adminResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// A blocked hard delete is a documented 409 payload, not a problem document:
	// the SPA renders the blocker list so the admin knows which teams to transfer
	// (#455). It travels as an error because the strict server's handler
	// signature only carries a response object OR an error.
	var blockedErr *adminDeleteBlockedError
	if errors.As(err, &blockedErr) {
		writeJSON(w, http.StatusConflict, toGenDeleteBlockedResponse(blockedErr), s.logger)
		return
	}

	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With("error", err).Error("Admin strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(adminMsgInternalError))
}
