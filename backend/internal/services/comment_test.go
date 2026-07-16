package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

// TestComments_NotInEmbeddingWorker guards the "comments are never searchable /
// embedded" invariant by construction: CommentService takes no event publisher,
// and no comment event type is registered with the embedding worker. If someone
// later adds comment embedding (e.g. by copying the feed-reply pattern), this
// fails.
func TestComments_NotInEmbeddingWorker(t *testing.T) {
	for _, et := range events.EmbeddingEventTypes() {
		assert.NotContains(t, et, "comment",
			"comments must not be added to the embedding worker's event types")
	}
}

const (
	commentTeamID  = resRBACTeamID
	commentCaller  = resRBACCaller
	commentOther   = resRBACOther
	commentResID   = "resource-1"
	commentID      = "comment-1"
	commentResType = models.CommentResourceTypeArtifact
	commentBody    = "a useful note"
	commentNewBody = "an edited note"
	commentBadType = "project"
	commentTooLong = 10001
)

// membershipStub is a minimal TeamServiceInterface double: only
// IsUserMemberOfTeam is exercised by comment reads; any other call panics via
// the embedded nil interface, surfacing an unexpected dependency.
type membershipStub struct {
	TeamServiceInterface
	isMember bool
	err      error
}

func (m membershipStub) IsUserMemberOfTeam(_ context.Context, _, _ string) (bool, error) {
	return m.isMember, m.err
}

func newCommentService(
	t *testing.T, repo repositories.CommentRepository, team TeamServiceInterface, authz AuthorizationServiceInterface,
) *CommentService {
	t.Helper()
	logger, _ := logtest.New()
	return NewCommentService(repo, team, authz, logger)
}

func TestCommentService_Create_ContentValidation(t *testing.T) {
	cases := []struct {
		name    string
		rtype   string
		content string
	}{
		{"empty content", commentResType, ""},
		{"whitespace-only content", commentResType, "   \t\n"},
		{"content too long", commentResType, strings.Repeat("a", commentTooLong)},
		{"invalid resource type", commentBadType, commentBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockCommentRepository(t)
			svc := newCommentService(t, repo, nil, allowAllAuthz{})

			_, err := svc.Create(context.Background(), commentCaller, commentTeamID, &models.CreateCommentRequest{
				ResourceType: tc.rtype,
				ResourceID:   commentResID,
				Content:      tc.content,
			})

			require.Error(t, err)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "ResourceExists", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCommentService_Create_ResourceMustExist(t *testing.T) {
	repo := mocks.NewMockCommentRepository(t)
	repo.EXPECT().ResourceExists(mock.Anything, commentTeamID, commentResType, commentResID).
		Return(false, nil).Once()
	svc := newCommentService(t, repo, nil, allowAllAuthz{})

	_, err := svc.Create(context.Background(), commentCaller, commentTeamID, &models.CreateCommentRequest{
		ResourceType: commentResType,
		ResourceID:   commentResID,
		Content:      commentBody,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource not found")
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCommentService_Create_NonMemberDenied(t *testing.T) {
	repo := mocks.NewMockCommentRepository(t)
	svc := newCommentService(t, repo, nil, authzForRole(t, ""))

	_, err := svc.Create(context.Background(), commentCaller, commentTeamID, &models.CreateCommentRequest{
		ResourceType: commentResType,
		ResourceID:   commentResID,
		Content:      commentBody,
	})

	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertNotCalled(t, "ResourceExists", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// Create is open to every role — the matrix grants resource.create to all.
func TestCommentService_Create_AnyMemberMayComment(t *testing.T) {
	for _, role := range []models.TeamMemberRole{
		models.TeamMemberRoleMember, models.TeamMemberRoleAdmin, models.TeamMemberRoleOwner,
	} {
		t.Run(string(role), func(t *testing.T) {
			repo := mocks.NewMockCommentRepository(t)
			repo.EXPECT().ResourceExists(mock.Anything, commentTeamID, commentResType, commentResID).
				Return(true, nil).Once()
			repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(c *models.Comment) bool {
				return c.TeamID == commentTeamID && c.UserID == commentCaller &&
					c.ResourceType == commentResType && c.ResourceID == commentResID && c.Content == commentBody
			})).Return(nil).Once()
			svc := newCommentService(t, repo, nil, authzForRole(t, role))

			created, err := svc.Create(context.Background(), commentCaller, commentTeamID, &models.CreateCommentRequest{
				ResourceType: commentResType,
				ResourceID:   commentResID,
				Content:      "  " + commentBody + "  ", // trimmed by the service
			})

			require.NoError(t, err)
			assert.Equal(t, commentBody, created.Content)
			assert.Equal(t, commentCaller, created.UserID)
		})
	}
}

// Edit is strictly author-only for EVERY role: even an admin/owner cannot edit
// another member's comment (there is no *.update.any for comments).
func TestCommentService_Update_AuthorOnly(t *testing.T) {
	cases := []struct {
		name    string
		role    models.TeamMemberRole
		author  string
		allowed bool
	}{
		{"member edits own", models.TeamMemberRoleMember, commentCaller, true},
		{"member cannot edit another's", models.TeamMemberRoleMember, commentOther, false},
		{"admin edits own", models.TeamMemberRoleAdmin, commentCaller, true},
		{"admin cannot edit another's", models.TeamMemberRoleAdmin, commentOther, false},
		{"owner edits own", models.TeamMemberRoleOwner, commentCaller, true},
		{"owner cannot edit another's", models.TeamMemberRoleOwner, commentOther, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockCommentRepository(t)
			repo.EXPECT().GetByID(mock.Anything, commentTeamID, commentID).
				Return(&models.Comment{ID: commentID, TeamID: commentTeamID, UserID: tc.author}, nil).Once()
			if tc.allowed {
				repo.EXPECT().UpdateContent(mock.Anything, commentTeamID, commentID, commentNewBody).
					Return(&models.Comment{ID: commentID, TeamID: commentTeamID, UserID: tc.author, Content: commentNewBody}, nil).Once()
			}
			// Update never consults the matrix (author-only), so authz allows all.
			svc := newCommentService(t, repo, nil, allowAllAuthz{})

			updated, err := svc.Update(context.Background(), commentCaller, commentTeamID, commentID,
				&models.UpdateCommentRequest{Content: commentNewBody})

			if tc.allowed {
				require.NoError(t, err)
				assert.Equal(t, commentNewBody, updated.Content)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "UpdateContent", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCommentService_Update_ContentValidation(t *testing.T) {
	repo := mocks.NewMockCommentRepository(t)
	svc := newCommentService(t, repo, nil, allowAllAuthz{})

	_, err := svc.Update(context.Background(), commentCaller, commentTeamID, commentID,
		&models.UpdateCommentRequest{Content: "   "})

	require.Error(t, err)
	repo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpdateContent", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Delete is own-vs-any: members delete only their own; Admin/Owner delete any.
func TestCommentService_Delete_OwnVsAny(t *testing.T) {
	for _, tc := range deleteOwnVsAnyCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockCommentRepository(t)
			repo.EXPECT().GetByID(mock.Anything, commentTeamID, commentID).
				Return(&models.Comment{ID: commentID, TeamID: commentTeamID, UserID: tc.ownerID}, nil).Once()
			if tc.allowed {
				repo.EXPECT().Delete(mock.Anything, commentTeamID, commentID).Return(nil).Once()
			}
			svc := newCommentService(t, repo, nil, authzForRole(t, tc.role))

			err := svc.Delete(context.Background(), commentCaller, commentTeamID, commentID)

			if tc.allowed {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrPermissionDenied)
			repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestCommentService_Delete_NotFound(t *testing.T) {
	repo := mocks.NewMockCommentRepository(t)
	repo.EXPECT().GetByID(mock.Anything, commentTeamID, commentID).
		Return(nil, repositories.ErrCommentNotFound).Once()
	svc := newCommentService(t, repo, nil, allowAllAuthz{})

	err := svc.Delete(context.Background(), commentCaller, commentTeamID, commentID)

	assert.ErrorIs(t, err, repositories.ErrCommentNotFound)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
}

func TestCommentService_ListByResource_ClampsAndRequiresMembership(t *testing.T) {
	t.Run("non-member denied", func(t *testing.T) {
		repo := mocks.NewMockCommentRepository(t)
		svc := newCommentService(t, repo, membershipStub{isMember: false}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), commentCaller, commentTeamID, commentResType, commentResID, 1, 5)

		require.Error(t, err)
		repo.AssertNotCalled(t, "ListByResource",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("invalid resource type", func(t *testing.T) {
		repo := mocks.NewMockCommentRepository(t)
		svc := newCommentService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		_, err := svc.ListByResource(context.Background(), commentCaller, commentTeamID, commentBadType, commentResID, 1, 5)

		require.Error(t, err)
	})

	t.Run("clamps page and limit", func(t *testing.T) {
		repo := mocks.NewMockCommentRepository(t)
		// page<=0 -> 1, limit>100 -> 100.
		repo.EXPECT().ListByResource(mock.Anything, commentTeamID, commentResType, commentResID, 1, 100).
			Return([]models.Comment{{ID: commentID}}, 1, nil).Once()
		svc := newCommentService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		resp, err := svc.ListByResource(context.Background(), commentCaller, commentTeamID, commentResType, commentResID, 0, 500)

		require.NoError(t, err)
		assert.Equal(t, 1, resp.Page)
		assert.Equal(t, 100, resp.PerPage)
		assert.Equal(t, 1, resp.TotalCount)
		assert.Equal(t, 1, resp.TotalPages)
		assert.Len(t, []models.Comment(resp.Comments), 1)
	})
}

func TestCommentService_ListRecentByTeam_ClampsAndRequiresMembership(t *testing.T) {
	t.Run("non-member denied", func(t *testing.T) {
		repo := mocks.NewMockCommentRepository(t)
		svc := newCommentService(t, repo, membershipStub{isMember: false}, allowAllAuthz{})

		_, err := svc.ListRecentByTeam(context.Background(), commentCaller, commentTeamID, 5)

		require.Error(t, err)
		repo.AssertNotCalled(t, "ListRecentByTeam", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("defaults and caps the limit", func(t *testing.T) {
		repo := mocks.NewMockCommentRepository(t)
		repo.EXPECT().ListRecentByTeam(mock.Anything, commentTeamID, commentDefaultRecentLimit).
			Return([]models.CommentActivity{}, nil).Once()
		svc := newCommentService(t, repo, membershipStub{isMember: true}, allowAllAuthz{})

		_, err := svc.ListRecentByTeam(context.Background(), commentCaller, commentTeamID, 0)
		require.NoError(t, err)

		repo2 := mocks.NewMockCommentRepository(t)
		repo2.EXPECT().ListRecentByTeam(mock.Anything, commentTeamID, commentMaxListLimit).
			Return([]models.CommentActivity{}, nil).Once()
		svc2 := newCommentService(t, repo2, membershipStub{isMember: true}, allowAllAuthz{})

		_, err = svc2.ListRecentByTeam(context.Background(), commentCaller, commentTeamID, 9999)
		require.NoError(t, err)
	})
}
