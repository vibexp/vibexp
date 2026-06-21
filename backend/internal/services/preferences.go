package services

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// UserPreferencesService handles user preferences business logic
type UserPreferencesService struct {
	repo repositories.UserPreferencesRepository
}

// NewUserPreferencesService creates a new UserPreferencesService
func NewUserPreferencesService(repo repositories.UserPreferencesRepository) *UserPreferencesService {
	return &UserPreferencesService{repo: repo}
}

// GetPreferences retrieves user preferences, returning defaults if none exist
func (s *UserPreferencesService) GetPreferences(
	ctx context.Context, userID string,
) (*models.PreferencesResponse, error) {
	prefs, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if prefs == nil {
		// Return default preferences if none exist
		return &models.PreferencesResponse{
			Preferences: models.DefaultPreferences(),
		}, nil
	}

	// Legacy detection heuristic: rows written before the notifications sub-tree
	// existed (pre-PR #1253) will have both an empty Channels struct and nil Types.
	// This is safe to use as a sentinel because UpdatePreferences always ensures
	// Types is non-nil before persisting, so a legitimately user-customized row
	// will never have nil Types. However, if a future write path bypasses
	// UpdatePreferences and stores {empty Channels, nil Types}, the next read will
	// silently overwrite with defaults — any such path must ensure Types is populated.
	if prefs.Preferences.Notifications.Channels == (models.NotificationChannelPreferences{}) &&
		prefs.Preferences.Notifications.Types == nil {
		prefs.Preferences.Notifications = models.DefaultNotificationPreferences()
	}

	return &models.PreferencesResponse{
		Preferences: prefs.Preferences,
		UpdatedAt:   prefs.UpdatedAt,
	}, nil
}

// UpdatePreferences updates user preferences
func (s *UserPreferencesService) UpdatePreferences(
	ctx context.Context, userID string, req models.UpdatePreferencesRequest,
) (*models.PreferencesResponse, error) {
	// Get existing preferences or use defaults
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var prefs models.UserPreferences
	if existing != nil {
		prefs = *existing
	} else {
		prefs = models.UserPreferences{
			UserID:      userID,
			Preferences: models.DefaultPreferences(),
		}
	}

	// Apply updates
	if req.EmailNotification != nil {
		// Account security is always enabled and cannot be changed
		prefs.Preferences.EmailNotification.PlatformAnnouncement = req.EmailNotification.PlatformAnnouncement
		prefs.Preferences.EmailNotification.AccountSecurity = true // Always enforce as true
		prefs.Preferences.EmailNotification.NewFeature = req.EmailNotification.NewFeature
		prefs.Preferences.EmailNotification.MarketingPromotional = req.EmailNotification.MarketingPromotional
	}

	if req.Notifications != nil {
		// Full-replace semantics: the entire Notifications block is replaced with
		// the value from the request. This is intentional and matches the frontend
		// behaviour of always sending all notification fields together. Unlike the
		// EmailNotification block above (which is field-by-field), no partial-update
		// merge is performed here.

		// n is a top-level struct copy; this is sufficient because n.Types is
		// immediately reassigned (defaulted) if nil, and is never mutated in
		// place — the assignment prefs.Preferences.Notifications = n replaces the
		// entire map reference.
		n := *req.Notifications
		if n.Types == nil {
			n.Types = models.DefaultNotificationPreferences().Types
		}

		validEmailFreqs := map[string]bool{"instant": true, "digest": true, "none": true}
		for typeName, typePref := range n.Types {
			if typePref.Email != "" && !validEmailFreqs[typePref.Email] {
				return nil, fmt.Errorf(
					"invalid email frequency %q for type %q: must be instant, digest, or none",
					typePref.Email, typeName,
				)
			}
		}

		prefs.Preferences.Notifications = n
	}

	// Save preferences
	if err := s.repo.Upsert(ctx, &prefs); err != nil {
		return nil, err
	}

	return &models.PreferencesResponse{
		Preferences: prefs.Preferences,
		UpdatedAt:   prefs.UpdatedAt,
	}, nil
}
