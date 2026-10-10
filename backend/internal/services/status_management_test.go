package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testStatusUserID = "user-1"
	testStatusRowID  = "status-1"
)

var errStatusBoom = errors.New("connection reset")

// statusManagementHarness runs the REAL authorization and audit services over
// repository mocks, so a test states the caller's role and reads back the
// audit rows that would be stored, rather than asserting on a double of either.
type statusManagementHarness struct {
	svc     *StatusManagementService
	repo    *repomocks.MockTeamStatusRepository
	members *repomocks.MockTeamMemberRepository
	audits  *repomocks.MockTeamSettingsAuditRepository
	// recorded collects every audit entry that reached the repository.
	recorded []*models.TeamSettingsAudit
}

func newStatusManagementHarness(t *testing.T) *statusManagementHarness {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	h := &statusManagementHarness{
		repo:    repomocks.NewMockTeamStatusRepository(t),
		members: repomocks.NewMockTeamMemberRepository(t),
		audits:  repomocks.NewMockTeamSettingsAuditRepository(t),
	}
	authorizer := NewAuthorizationService(h.members, logger)
	audit := NewTeamSettingsAuditService(
		h.audits, authorizer,
		repomocks.NewMockUserRepository(t), repomocks.NewMockTeamRepository(t), logger,
	)
	h.svc = NewStatusManagementService(h.repo, NewStatusCatalogService(h.repo), authorizer, audit, logger)
	return h
}

// as makes the caller a team member with the given role.
func (h *statusManagementHarness) as(role models.TeamMemberRole) *statusManagementHarness {
	h.members.EXPECT().GetByTeamAndUser(mock.Anything, testStatusTeamID, testStatusUserID).
		Return(&models.TeamMember{Role: role}, nil)
	return h
}

// expectAudit lets exactly one audit entry be stored and captures it.
func (h *statusManagementHarness) expectAudit() {
	h.audits.EXPECT().Append(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entry *models.TeamSettingsAudit) error {
			h.recorded = append(h.recorded, entry)
			return nil
		}).Once()
}

// auditDetail decodes the single recorded entry's detail.
func (h *statusManagementHarness) auditDetail(t *testing.T) map[string]interface{} {
	t.Helper()
	require.Len(t, h.recorded, 1, "exactly one audit entry per successful write")
	entry := h.recorded[0]
	assert.Equal(t, testStatusTeamID, entry.TeamID)
	assert.Equal(t, models.SettingsAuditSurfaceCustomStatuses, entry.Surface)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testStatusUserID, *entry.ActorUserID)
	assert.Nil(t, entry.SourceTeamID, "a custom status change has no source team")

	var detail map[string]interface{}
	require.NoError(t, json.Unmarshal(entry.Detail, &detail))
	return detail
}

func createParams(resourceType, slug, name string) CreateStatusParams {
	return CreateStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID,
		ResourceType: resourceType, Slug: slug, Name: name,
	}
}

func TestStatusManagementService_List_CatalogWithUsageCounts(t *testing.T) {
	h := newStatusManagementHarness(t)
	h.repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
		Return([]models.TeamStatus{
			customStatus(testStatusRowID, models.StatusResourceTypeArtifact, "in-review", "In review"),
		}, nil)
	h.repo.EXPECT().UsageCounts(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
		Return(map[string]int{"active": 4, "in-review": 2, "orphaned": 9}, nil)

	got, err := h.svc.List(context.Background(), testStatusTeamID, models.StatusResourceTypeArtifact)
	require.NoError(t, err)

	type row struct {
		slug     string
		isSystem bool
		usage    int
	}
	rows := make([]row, 0, len(got))
	for _, status := range got {
		assert.Equal(t, models.StatusResourceTypeArtifact, status.ResourceType)
		rows = append(rows, row{status.Slug, status.IsSystem, status.UsageCount})
	}
	// System statuses first, each with its own count; a status nothing uses
	// counts zero; a slug in use that is not in the catalog is not listed.
	assert.Equal(t, []row{
		{"active", true, 4}, {"draft", true, 0}, {"archived", true, 0}, {"in-review", false, 2},
	}, rows)
	require.NotNil(t, got[3].ID)
	assert.Equal(t, testStatusRowID, *got[3].ID)
	assert.Nil(t, got[0].ID)
}

// The list is open to every member: it resolves no role and writes no audit
// entry. The harness's member and audit mocks carry no expectation, so either
// call would fail the test.
func TestStatusManagementService_List_AuthorizesAndAuditsNothing(t *testing.T) {
	h := newStatusManagementHarness(t)
	h.repo.EXPECT().List(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).
		Return([]models.TeamStatus{}, nil)
	h.repo.EXPECT().UsageCounts(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).
		Return(map[string]int{}, nil)

	_, err := h.svc.List(context.Background(), testStatusTeamID, models.StatusResourceTypeMemory)
	require.NoError(t, err)
}

func TestStatusManagementService_List_Errors(t *testing.T) {
	t.Run("an unsupported resource type runs no query", func(t *testing.T) {
		h := newStatusManagementHarness(t)
		_, err := h.svc.List(context.Background(), testStatusTeamID, "artifacts")
		require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)
	})

	t.Run("a usage count failure is returned", func(t *testing.T) {
		h := newStatusManagementHarness(t)
		h.repo.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).Return([]models.TeamStatus{}, nil)
		h.repo.EXPECT().UsageCounts(mock.Anything, mock.Anything, mock.Anything).Return(nil, errStatusBoom)

		_, err := h.svc.List(context.Background(), testStatusTeamID, models.StatusResourceTypePrompt)
		require.ErrorIs(t, err, errStatusBoom)
	})
}

func TestStatusManagementService_Create_StoresAuditsAndReturnsTheRow(t *testing.T) {
	for _, role := range []models.TeamMemberRole{models.TeamMemberRoleOwner, models.TeamMemberRoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			h := newStatusManagementHarness(t).as(role)
			h.repo.EXPECT().Count(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
				Return(MaxCustomStatusesPerResourceType-1, nil)
			h.repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row *models.TeamStatus) bool {
				return row.TeamID == testStatusTeamID &&
					row.ResourceType == models.StatusResourceTypeArtifact &&
					row.Slug == "in-review" && row.Name == "In review" &&
					row.CreatedBy != nil && *row.CreatedBy == testStatusUserID
			})).RunAndReturn(func(_ context.Context, row *models.TeamStatus) error {
				row.ID = testStatusRowID
				row.CreatedAt, row.UpdatedAt = testStatusCreatedAt, testStatusCreatedAt
				return nil
			})
			h.repo.EXPECT().UsageCounts(mock.Anything, testStatusTeamID, models.StatusResourceTypeArtifact).
				Return(map[string]int{"active": 3}, nil)
			h.expectAudit()

			// The name is stored trimmed; the 20th status is still allowed.
			got, err := h.svc.Create(context.Background(),
				createParams(models.StatusResourceTypeArtifact, "in-review", "  In review \t"))
			require.NoError(t, err)

			require.NotNil(t, got.ID)
			assert.Equal(t, testStatusRowID, *got.ID)
			assert.Equal(t, "in-review", got.Slug)
			assert.Equal(t, "In review", got.Name)
			assert.False(t, got.IsSystem)
			assert.True(t, got.IsLive, "a custom status is always live")
			assert.Zero(t, got.UsageCount)
			require.NotNil(t, got.CreatedAt)
			assert.Equal(t, testStatusCreatedAt, *got.CreatedAt)

			detail := h.auditDetail(t)
			assert.Equal(t, map[string]interface{}{
				"action": "created", "resource_type": "artifact", "slug": "in-review", "name": "In review",
			}, detail)
			require.NotNil(t, h.recorded[0].CreatedResourceID)
			assert.Equal(t, testStatusRowID, *h.recorded[0].CreatedResourceID)
		})
	}
}

// A member holds no team.settings.update, and a non-member holds nothing: both
// are refused before any input is read or any row is touched. The repository
// and audit mocks carry no expectation, so a write would fail the test.
func TestStatusManagementService_Writes_RequireTeamSettingsUpdate(t *testing.T) {
	writes := map[string]func(h *statusManagementHarness) error{
		"create": func(h *statusManagementHarness) error {
			_, err := h.svc.Create(context.Background(),
				createParams(models.StatusResourceTypeArtifact, "in-review", "In review"))
			return err
		},
		"rename": func(h *statusManagementHarness) error {
			_, err := h.svc.Rename(context.Background(), RenameStatusParams{
				TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "New",
			})
			return err
		},
		"delete": func(h *statusManagementHarness) error {
			_, err := h.svc.Delete(context.Background(), DeleteStatusParams{
				TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID,
			})
			return err
		},
	}

	for name, write := range writes {
		t.Run("a member cannot "+name, func(t *testing.T) {
			h := newStatusManagementHarness(t).as(models.TeamMemberRoleMember)
			require.ErrorIs(t, write(h), ErrPermissionDenied)
			assert.Empty(t, h.recorded)
		})

		t.Run("a non-member cannot "+name, func(t *testing.T) {
			h := newStatusManagementHarness(t)
			h.members.EXPECT().GetByTeamAndUser(mock.Anything, testStatusTeamID, testStatusUserID).
				Return(nil, repositories.ErrTeamMemberNotFound)
			require.ErrorIs(t, write(h), ErrPermissionDenied)
		})

		t.Run("a role lookup failure on "+name+" is not a denial", func(t *testing.T) {
			h := newStatusManagementHarness(t)
			h.members.EXPECT().GetByTeamAndUser(mock.Anything, testStatusTeamID, testStatusUserID).
				Return(nil, errStatusBoom)
			err := write(h)
			require.ErrorIs(t, err, errStatusBoom)
			assert.NotErrorIs(t, err, ErrPermissionDenied)
		})
	}
}

// The permission the writes ask for is the one the matrix grants to owners and
// admins only; if the matrix changes, the role tests above change meaning.
func TestStatusManagementService_WritePermissionIsOwnerAndAdminOnly(t *testing.T) {
	assert.True(t, authz.Allowed(models.TeamMemberRoleOwner, authz.TeamSettingsUpdate))
	assert.True(t, authz.Allowed(models.TeamMemberRoleAdmin, authz.TeamSettingsUpdate))
	assert.False(t, authz.Allowed(models.TeamMemberRoleMember, authz.TeamSettingsUpdate))
}

func TestStatusManagementService_Create_RejectsInvalidInput(t *testing.T) {
	cases := map[string]struct {
		resourceType, slug, name string
		want                     error
	}{
		"plural resource type":   {"artifacts", "in-review", "In review", ErrStatusResourceTypeUnsupported},
		"empty resource type":    {"", "in-review", "In review", ErrStatusResourceTypeUnsupported},
		"empty slug":             {"artifact", "", "In review", ErrStatusSlugInvalid},
		"uppercase slug":         {"artifact", "In-Review", "In review", ErrStatusSlugInvalid},
		"slug with a space":      {"artifact", "in review", "In review", ErrStatusSlugInvalid},
		"slug with underscore":   {"artifact", "in_review", "In review", ErrStatusSlugInvalid},
		"leading hyphen":         {"artifact", "-review", "In review", ErrStatusSlugInvalid},
		"trailing hyphen":        {"artifact", "review-", "In review", ErrStatusSlugInvalid},
		"doubled hyphen":         {"artifact", "in--review", "In review", ErrStatusSlugInvalid},
		"padded slug":            {"artifact", " in-review ", "In review", ErrStatusSlugInvalid},
		"51-character slug":      {"artifact", strings.Repeat("a", 51), "In review", ErrStatusSlugTooLong},
		"empty name":             {"artifact", "in-review", "", ErrStatusNameRequired},
		"whitespace-only name":   {"artifact", "in-review", " \t\n ", ErrStatusNameRequired},
		"51-character name":      {"artifact", "in-review", strings.Repeat("n", 51), ErrStatusNameTooLong},
		"51 multi-byte chars":    {"artifact", "in-review", strings.Repeat("é", 51), ErrStatusNameTooLong},
		"resource type is first": {"widget", "BAD SLUG", "", ErrStatusResourceTypeUnsupported},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Only the role lookup is expected: invalid input never reaches
			// the status repository or the audit log.
			h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)

			got, err := h.svc.Create(context.Background(), createParams(tc.resourceType, tc.slug, tc.name))
			require.ErrorIs(t, err, tc.want)
			assert.Nil(t, got)
		})
	}
}

func TestStatusManagementService_Create_AcceptsTheLimitsExactly(t *testing.T) {
	h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
	slug := strings.Repeat("a", 50)
	name := strings.Repeat("é", 50) // 50 characters, 100 bytes
	h.repo.EXPECT().Count(mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
	h.repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(row *models.TeamStatus) bool {
		return row.Slug == slug && row.Name == name
	})).Return(nil)
	h.repo.EXPECT().UsageCounts(mock.Anything, mock.Anything, mock.Anything).Return(map[string]int{}, nil)
	h.expectAudit()

	_, err := h.svc.Create(context.Background(), createParams(models.StatusResourceTypePrompt, slug, name))
	require.NoError(t, err)
}

// Every system word is reserved on every resource type, including the types
// that do not use it themselves: `published` is not an artifact status, and a
// custom artifact status may still not take the name.
func TestStatusManagementService_Create_ReservedSlugsOnEveryResourceType(t *testing.T) {
	require.Len(t, models.ReservedStatusSlugs, 5)
	for _, resourceType := range models.StatusResourceTypes {
		for _, slug := range models.ReservedStatusSlugs {
			t.Run(resourceType+"/"+slug, func(t *testing.T) {
				h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)

				_, err := h.svc.Create(context.Background(), createParams(resourceType, slug, "Mine"))
				require.ErrorIs(t, err, ErrStatusReserved)
			})
		}
	}
}

func TestStatusManagementService_Create_Limit(t *testing.T) {
	t.Run("the 21st status is refused without an insert", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().Count(mock.Anything, testStatusTeamID, models.StatusResourceTypeMemory).
			Return(MaxCustomStatusesPerResourceType, nil)

		_, err := h.svc.Create(context.Background(),
			createParams(models.StatusResourceTypeMemory, "in-review", "In review"))
		require.ErrorIs(t, err, ErrStatusLimitReached)
		assert.Empty(t, h.recorded)
	})

	t.Run("the limit is 20", func(t *testing.T) {
		assert.Equal(t, 20, MaxCustomStatusesPerResourceType)
	})

	t.Run("a count failure is returned", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().Count(mock.Anything, mock.Anything, mock.Anything).Return(0, errStatusBoom)

		_, err := h.svc.Create(context.Background(),
			createParams(models.StatusResourceTypeMemory, "in-review", "In review"))
		require.ErrorIs(t, err, errStatusBoom)
	})
}

func TestStatusManagementService_Create_DuplicateWritesNoAudit(t *testing.T) {
	h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
	h.repo.EXPECT().Count(mock.Anything, mock.Anything, mock.Anything).Return(1, nil)
	h.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(repositories.ErrTeamStatusExists)

	_, err := h.svc.Create(context.Background(),
		createParams(models.StatusResourceTypeArtifact, "in-review", "In review"))
	require.ErrorIs(t, err, repositories.ErrTeamStatusExists)
	assert.Empty(t, h.recorded, "a failed write appends no audit entry")
}

func TestStatusManagementService_Rename(t *testing.T) {
	params := RenameStatusParams{
		TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "  Under review ",
	}
	stored := customStatus(testStatusRowID, models.StatusResourceTypeBlueprint, "in-review", "In review")

	t.Run("renames, keeps the slug and audits the previous name", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
		renamed := stored
		renamed.Name = "Under review"
		h.repo.EXPECT().GetByID(mock.Anything, testStatusTeamID, testStatusRowID).Return(&stored, nil)
		h.repo.EXPECT().UpdateName(mock.Anything, testStatusTeamID, testStatusRowID, "Under review").
			Return(&renamed, nil)
		h.repo.EXPECT().UsageCounts(mock.Anything, testStatusTeamID, models.StatusResourceTypeBlueprint).
			Return(map[string]int{"in-review": 6}, nil)
		h.expectAudit()

		got, err := h.svc.Rename(context.Background(), params)
		require.NoError(t, err)
		assert.Equal(t, "Under review", got.Name)
		assert.Equal(t, "in-review", got.Slug)
		assert.Equal(t, 6, got.UsageCount)

		assert.Equal(t, map[string]interface{}{
			"action": "renamed", "resource_type": "blueprint", "slug": "in-review",
			"name": "Under review", "previous_name": "In review",
		}, h.auditDetail(t))
		assert.Nil(t, h.recorded[0].CreatedResourceID, "a rename creates nothing")
	})

	for name, tc := range map[string]struct {
		name string
		want error
	}{
		"empty name":        {"   ", ErrStatusNameRequired},
		"51-character name": {strings.Repeat("n", 51), ErrStatusNameTooLong},
	} {
		t.Run(name+" is rejected before any lookup", func(t *testing.T) {
			h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
			bad := params
			bad.Name = tc.name

			_, err := h.svc.Rename(context.Background(), bad)
			require.ErrorIs(t, err, tc.want)
		})
	}

	t.Run("an unknown id is not found and writes no audit", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().GetByID(mock.Anything, testStatusTeamID, testStatusRowID).
			Return(nil, repositories.ErrTeamStatusNotFound)

		_, err := h.svc.Rename(context.Background(), params)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
		assert.Empty(t, h.recorded)
	})

	t.Run("a row deleted between the read and the update is not found", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)
		h.repo.EXPECT().UpdateName(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, repositories.ErrTeamStatusNotFound)

		_, err := h.svc.Rename(context.Background(), params)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
		assert.Empty(t, h.recorded)
	})
}

func TestStatusManagementService_Delete(t *testing.T) {
	params := DeleteStatusParams{TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID}

	for _, resourceType := range models.StatusResourceTypes {
		t.Run(resourceType+" resources go to the type's default", func(t *testing.T) {
			h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
			stored := customStatus(testStatusRowID, resourceType, "in-review", "In review")
			wantDefault := models.DefaultStatusFor(resourceType)
			h.repo.EXPECT().GetByID(mock.Anything, testStatusTeamID, testStatusRowID).Return(&stored, nil)
			h.repo.EXPECT().DeleteAndReassign(mock.Anything, testStatusTeamID, testStatusRowID, wantDefault).
				Return(&stored, 12, nil)
			h.expectAudit()

			got, err := h.svc.Delete(context.Background(), params)
			require.NoError(t, err)
			assert.Equal(t, &StatusDeletion{ReassignedCount: 12, ReassignedTo: wantDefault}, got)

			assert.Equal(t, map[string]interface{}{
				"action": "deleted", "resource_type": resourceType, "slug": "in-review",
				"name": "In review", "reassigned_count": float64(12), "reassigned_to": wantDefault,
			}, h.auditDetail(t))
		})
	}

	t.Run("prompts default to draft, everything else to active", func(t *testing.T) {
		assert.Equal(t, models.StatusDraft, models.DefaultStatusFor(models.StatusResourceTypePrompt))
		assert.Equal(t, models.StatusActive, models.DefaultStatusFor(models.StatusResourceTypeMemory))
	})

	t.Run("an unknown id is not found and writes no audit", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
		h.repo.EXPECT().GetByID(mock.Anything, testStatusTeamID, testStatusRowID).
			Return(nil, repositories.ErrTeamStatusNotFound)

		_, err := h.svc.Delete(context.Background(), params)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
		assert.Empty(t, h.recorded)
	})

	t.Run("a failed delete writes no audit", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
		stored := customStatus(testStatusRowID, models.StatusResourceTypeArtifact, "in-review", "In review")
		h.repo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)
		h.repo.EXPECT().DeleteAndReassign(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, 0, errStatusBoom)

		_, err := h.svc.Delete(context.Background(), params)
		require.ErrorIs(t, err, errStatusBoom)
		assert.Empty(t, h.recorded)
	})

	t.Run("a stored row of an unknown resource type is refused, not reassigned", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleAdmin)
		stored := customStatus(testStatusRowID, "widget", "in-review", "In review")
		h.repo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)

		_, err := h.svc.Delete(context.Background(), params)
		require.ErrorIs(t, err, ErrStatusResourceTypeUnsupported)
	})
}

// The audit entry is written after the change is stored. When it cannot be
// written the caller gets the error -- the interface contract of
// TeamSettingsAuditServiceInterface.Record -- even though the change stands.
func TestStatusManagementService_AuditFailureIsReturned(t *testing.T) {
	stored := customStatus(testStatusRowID, models.StatusResourceTypeArtifact, "in-review", "In review")

	t.Run("create", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().Count(mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
		h.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		h.audits.EXPECT().Append(mock.Anything, mock.Anything).Return(errStatusBoom)

		got, err := h.svc.Create(context.Background(),
			createParams(models.StatusResourceTypeArtifact, "in-review", "In review"))
		require.ErrorIs(t, err, errStatusBoom)
		assert.Nil(t, got)
	})

	t.Run("rename", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)
		h.repo.EXPECT().UpdateName(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)
		h.audits.EXPECT().Append(mock.Anything, mock.Anything).Return(errStatusBoom)

		_, err := h.svc.Rename(context.Background(), RenameStatusParams{
			TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID, Name: "New",
		})
		require.ErrorIs(t, err, errStatusBoom)
	})

	t.Run("delete", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().GetByID(mock.Anything, mock.Anything, mock.Anything).Return(&stored, nil)
		h.repo.EXPECT().DeleteAndReassign(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&stored, 1, nil)
		h.audits.EXPECT().Append(mock.Anything, mock.Anything).Return(errStatusBoom)

		got, err := h.svc.Delete(context.Background(), DeleteStatusParams{
			TeamID: testStatusTeamID, UserID: testStatusUserID, ID: testStatusRowID,
		})
		require.ErrorIs(t, err, errStatusBoom)
		assert.Nil(t, got)
	})

	t.Run("a usage count failure after a stored write is returned", func(t *testing.T) {
		h := newStatusManagementHarness(t).as(models.TeamMemberRoleOwner)
		h.repo.EXPECT().Count(mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
		h.repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		h.repo.EXPECT().UsageCounts(mock.Anything, mock.Anything, mock.Anything).Return(nil, errStatusBoom)
		h.expectAudit()

		_, err := h.svc.Create(context.Background(),
			createParams(models.StatusResourceTypeArtifact, "in-review", "In review"))
		require.ErrorIs(t, err, errStatusBoom)
	})
}
