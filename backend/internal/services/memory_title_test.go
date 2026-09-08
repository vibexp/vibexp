package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The optional memory title (issue #911, epic #899). The interesting parts are
// the two documented semantics -- the 255-rune limit, enforced here because the
// `validate:` tags on this domain are inert and the generated binder ignores
// `maxLength`, and the three-state update where an absent key and an explicit
// null mean different things.

const memoryTitleTestProjectID = "550e8400-e29b-41d4-a716-446655440002"

func titlePtr(s string) *string { return &s }

func TestMemoryService_CreateMemory_Title(t *testing.T) {
	tests := []struct {
		name    string
		title   *string
		want    *string
		wantErr bool
	}{
		{name: "no title stays null", title: nil, want: nil},
		{name: "a title is stored", title: titlePtr("Deploy checklist"), want: titlePtr("Deploy checklist")},
		{name: "surrounding whitespace is trimmed", title: titlePtr("  Deploy  "), want: titlePtr("Deploy")},
		{name: "an empty title collapses to null", title: titlePtr(""), want: nil},
		{name: "a whitespace-only title collapses to null", title: titlePtr("   "), want: nil},
		{
			name:  "exactly the limit is accepted",
			title: titlePtr(strings.Repeat("x", services.MaxMemoryTitleLength)),
			want:  titlePtr(strings.Repeat("x", services.MaxMemoryTitleLength)),
		},
		{
			name:    "one rune over the limit is rejected",
			title:   titlePtr(strings.Repeat("x", services.MaxMemoryTitleLength+1)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repomocks.NewMockMemoryRepository(t)
			if !tt.wantErr {
				mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(m *models.Memory) bool {
					if tt.want == nil {
						return m.Title == nil
					}
					return m.Title != nil && *m.Title == *tt.want
				})).Return(nil).Once()
			}

			logger, _ := logtest.New()
			service := services.NewMemoryService(
				mockRepo, nil, permissiveAuthz(t), nil, logger, nil, nil, nil, nil, nil)
			memory, err := service.CreateMemory("user-123", "team-123", &models.CreateMemoryRequest{
				ProjectID: memoryTitleTestProjectID,
				Title:     tt.title,
				Text:      "some memory",
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, services.ErrInvalidMemoryTitle)
				return
			}
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, memory.Title)
			} else {
				require.NotNil(t, memory.Title)
				assert.Equal(t, *tt.want, *memory.Title)
			}
		})
	}
}

// A *string could not express these three cases: absent and null would both
// decode to nil. This is the reason models.OptionalString exists.
func TestMemoryService_UpdateMemory_TitleThreeStates(t *testing.T) {
	existingTitle := "Original title"

	tests := []struct {
		name    string
		title   models.OptionalString
		want    *string
		wantErr bool
	}{
		{name: "absent leaves the title unchanged", title: models.OptionalString{}, want: &existingTitle},
		{name: "explicit null clears the title", title: models.ClearedString(), want: nil},
		{name: "a value replaces the title", title: models.NewOptionalString("New title"), want: titlePtr("New title")},
		{name: "an empty value clears the title", title: models.NewOptionalString("  "), want: nil},
		{
			name:    "an over-long value is rejected",
			title:   models.NewOptionalString(strings.Repeat("x", services.MaxMemoryTitleLength+1)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := repomocks.NewMockMemoryRepository(t)
			existing := &models.Memory{
				ID: "memory-123", UserID: "user-123", TeamID: "team-123",
				ProjectID: memoryTitleTestProjectID, Text: "some memory",
				Title: titlePtr(existingTitle),
			}
			mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").
				Return(existing, nil).Once()
			if !tt.wantErr {
				mockRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
			}

			logger, _ := logtest.New()
			service := services.NewMemoryService(
				mockRepo, nil, permissiveAuthz(t), nil, logger, nil, nil, nil, nil, nil)
			memory, err := service.UpdateMemory("user-123", "team-123", "memory-123",
				&models.UpdateMemoryRequest{Title: tt.title})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, services.ErrInvalidMemoryTitle)
				return
			}
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, memory.Title)
			} else {
				require.NotNil(t, memory.Title)
				assert.Equal(t, *tt.want, *memory.Title)
			}
		})
	}
}

// The versioned content of a memory is its text (see applyAndPersistMemoryUpdate).
// A title-only edit is deliberately NOT a new content version: the snapshot
// records what the text was, and the text did not change.
func TestMemoryService_TitleOnlyEditCreatesNoContentVersion(t *testing.T) {
	mockRepo := repomocks.NewMockMemoryRepository(t)
	existing := &models.Memory{
		ID: "memory-123", UserID: "user-123", TeamID: "team-123",
		ProjectID: memoryTitleTestProjectID, Text: "some memory",
	}
	mockRepo.EXPECT().GetByID(mock.Anything, "user-123", mock.Anything, "memory-123").
		Return(existing, nil).Once()
	mockRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	versionSvc := servicemocks.NewMockContentVersionServiceInterface(t)
	logger, _ := logtest.New()
	service := services.NewMemoryService(
		mockRepo, nil, permissiveAuthz(t), nil, logger, versionSvc, nil, nil, nil, nil)

	_, err := service.UpdateMemory("user-123", "team-123", "memory-123",
		&models.UpdateMemoryRequest{Title: models.NewOptionalString("Just a title")})

	require.NoError(t, err)
	versionSvc.AssertNotCalled(t, "SnapshotIfChanged", mock.Anything, mock.Anything)
}
