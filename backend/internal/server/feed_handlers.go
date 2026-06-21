package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/pkg/events"
)

// ─────────────────────────────────────────────────────────────────────────────
// Feed CRUD handlers
// ─────────────────────────────────────────────────────────────────────────────

func (s *Server) handleCreateFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleCreateFeed",
		"user_id": userID,
		"team_id": teamID,
	}).Info("Create feed request received")

	var req models.CreateFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateFeedRequest(w, &req) {
		return
	}

	if !s.checkFeedResourceLimit(w, r.Context(), userID) {
		return
	}

	feed, err := s.container.FeedService().CreateFeed(r.Context(), userID, teamID, &req)
	if err != nil {
		s.handleFeedServiceError(w, "handleCreateFeed", userID, teamID, err)
		return
	}

	writeCreated(w, feed, s.logger)
}

func (s *Server) handleGetFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	feedID := chi.URLParam(r, "feed_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetFeed",
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
	}).Info("Get feed request received")

	if !isValidUUID(feedID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid feed_id format", http.StatusBadRequest)
		return
	}

	feed, err := s.container.FeedService().GetFeed(r.Context(), userID, teamID, feedID)
	if err != nil {
		s.handleFeedGetError(w, "handleGetFeed", userID, teamID, feedID, err)
		return
	}

	writeOK(w, feed, s.logger)
}

func (s *Server) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleListFeeds",
		"user_id": userID,
		"team_id": teamID,
	}).Info("List feeds request received")

	filters := s.buildFeedFilters(r, teamID)

	response, err := s.container.FeedService().ListFeeds(r.Context(), userID, filters)
	if err != nil {
		s.handleFeedServiceError(w, "handleListFeeds", userID, teamID, err)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleUpdateFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	feedID := chi.URLParam(r, "feed_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleUpdateFeed",
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
	}).Info("Update feed request received")

	if !isValidUUID(feedID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid feed_id format", http.StatusBadRequest)
		return
	}

	var req models.UpdateFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateFeedRequest(w, &req) {
		return
	}

	feed, err := s.container.FeedService().UpdateFeed(r.Context(), userID, teamID, feedID, &req)
	if err != nil {
		s.handleFeedGetError(w, "handleUpdateFeed", userID, teamID, feedID, err)
		return
	}

	writeOK(w, feed, s.logger)
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	feedID := chi.URLParam(r, "feed_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleDeleteFeed",
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
	}).Info("Delete feed request received")

	if !isValidUUID(feedID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid feed_id format", http.StatusBadRequest)
		return
	}

	err := s.container.FeedService().DeleteFeed(r.Context(), userID, teamID, feedID)
	if err != nil {
		s.handleFeedGetError(w, "handleDeleteFeed", userID, teamID, feedID, err)
		return
	}

	writeNoContent(w)
}

// ─────────────────────────────────────────────────────────────────────────────
// Feed item handlers
// ─────────────────────────────────────────────────────────────────────────────

func (s *Server) handleCreateFeedItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	feedID := chi.URLParam(r, "feed_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleCreateFeedItem",
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
	}).Info("Create feed item request received")

	if !isValidUUID(feedID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid feed_id format", http.StatusBadRequest)
		return
	}

	var req models.CreateFeedItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateFeedItemRequest(w, &req) {
		return
	}

	if !s.checkFeedItemResourceLimit(w, r.Context(), userID) {
		return
	}

	item, err := s.container.FeedItemService().CreateFeedItem(r.Context(), userID, teamID, feedID, &req)
	if err != nil {
		s.handleFeedItemServiceError(w, "handleCreateFeedItem", userID, teamID, feedID, err)
		return
	}

	writeCreated(w, item, s.logger)
}

func (s *Server) handleListFeedItems(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleListFeedItems",
		"user_id": userID,
		"team_id": teamID,
	}).Info("List feed items request received")

	filters, filterErr := s.buildFeedItemFilters(r, teamID, "")
	if filterErr != nil {
		writeErrorResponse(w, nil, "bad_request", filterErr.Error(), http.StatusBadRequest)
		return
	}

	response, err := s.container.FeedItemService().ListFeedItems(r.Context(), userID, filters)
	if err != nil {
		s.handleFeedItemServiceError(w, "handleListFeedItems", userID, teamID, "", err)
		return
	}

	enriched, err := s.container.FeedItemService().EnrichWithReplyCounts(r.Context(), teamID, response.Items)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to enrich feed items with reply counts; returning without counts")
	} else {
		response.Items = enriched
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleListFeedItemsByFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	feedID := chi.URLParam(r, "feed_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleListFeedItemsByFeed",
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
	}).Info("List feed items by feed request received")

	if !isValidUUID(feedID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid feed_id format", http.StatusBadRequest)
		return
	}

	filters, filterErr := s.buildFeedItemFilters(r, teamID, feedID)
	if filterErr != nil {
		writeErrorResponse(w, nil, "bad_request", filterErr.Error(), http.StatusBadRequest)
		return
	}

	response, err := s.container.FeedItemService().ListFeedItems(r.Context(), userID, filters)
	if err != nil {
		s.handleFeedItemServiceError(w, "handleListFeedItemsByFeed", userID, teamID, feedID, err)
		return
	}

	enriched, err := s.container.FeedItemService().EnrichWithReplyCounts(r.Context(), teamID, response.Items)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to enrich feed items with reply counts; returning without counts")
	} else {
		response.Items = enriched
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleGetFeedItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleGetFeedItem",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("Get feed item request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	item, err := s.container.FeedItemService().GetFeedItem(r.Context(), userID, teamID, itemID)
	if err != nil {
		s.handleFeedItemGetError(w, "handleGetFeedItem", userID, teamID, itemID, err)
		return
	}

	writeOK(w, item, s.logger)
}

func (s *Server) handleArchiveFeedItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleArchiveFeedItem",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("Archive feed item request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	err := s.container.FeedItemService().ArchiveFeedItem(r.Context(), userID, teamID, itemID)
	if err != nil {
		s.handleFeedItemGetError(w, "handleArchiveFeedItem", userID, teamID, itemID, err)
		return
	}

	writeNoContent(w)
}

func (s *Server) handleUnarchiveFeedItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleUnarchiveFeedItem",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("Unarchive feed item request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	err := s.container.FeedItemService().UnarchiveFeedItem(r.Context(), userID, teamID, itemID)
	if err != nil {
		s.handleFeedItemGetError(w, "handleUnarchiveFeedItem", userID, teamID, itemID, err)
		return
	}

	writeNoContent(w)
}

func (s *Server) handleDeleteFeedItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleDeleteFeedItem",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("Delete feed item request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	// Capture the item (for its poster) and its replies' posters BEFORE deletion. The DB
	// cascade removes reply rows but not their embedding rows, so we collect the keying
	// data now and clean the embeddings up after the hard delete succeeds. The item read is
	// fatal (it also authorizes existence and yields the poster id); the reply-poster read is
	// best-effort, so a transient failure there must not block the user's delete.
	item, err := s.container.FeedItemService().GetFeedItem(r.Context(), userID, teamID, itemID)
	if err != nil {
		s.handleFeedItemGetError(w, "handleDeleteFeedItem", userID, teamID, itemID, err)
		return
	}
	replyPosters := s.gatherReplyPostersForCleanup(r.Context(), userID, teamID, itemID)

	if err := s.container.FeedItemService().DeleteFeedItem(r.Context(), userID, teamID, itemID); err != nil {
		s.handleFeedItemGetError(w, "handleDeleteFeedItem", userID, teamID, itemID, err)
		return
	}

	s.deleteFeedItemEmbeddings(item, replyPosters)

	writeNoContent(w)
}

// deleteFeedItemEmbeddings removes the feed item's embedding row and each reply's
// embedding row. The delete is keyed solely on the entity, so it succeeds
// regardless of which member triggers it. Failures are non-fatal (logged Warn)
// since the item is already deleted.
func (s *Server) deleteFeedItemEmbeddings(
	item *models.FeedItem, replyPosters []repositories.FeedItemReplyPoster,
) {
	if err := s.container.EmbeddingService().DeleteEmbeddingsByEntity(
		"feed_item", item.ID,
	); err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleDeleteFeedItem",
			"user_id": item.PostedByUserID,
			"item_id": item.ID,
			"error":   fmt.Sprintf("%+v", err),
		}).Warn("Failed to delete feed item embeddings (non-fatal)")
	}

	for _, poster := range replyPosters {
		if err := s.container.EmbeddingService().DeleteEmbeddingsByEntity(
			"feed_item_reply", poster.ReplyID,
		); err != nil {
			s.logger.WithFields(logrus.Fields{
				"service":  "vibexp-api",
				"handler":  "handleDeleteFeedItem",
				"user_id":  poster.PostedByUserID,
				"reply_id": poster.ReplyID,
				"item_id":  item.ID,
				"error":    fmt.Sprintf("%+v", err),
			}).Warn("Failed to delete feed item reply embeddings (non-fatal)")
		}
	}
}

// gatherReplyPostersForCleanup lists the posters of an item's replies so their embeddings
// can be cleaned up after a hard delete. It is best-effort: on error it logs a warning and
// returns nil so a transient read failure never blocks the user's delete. The cleanup it
// feeds is itself non-fatal, and any reply embeddings left orphaned are reconciled by #1363.
func (s *Server) gatherReplyPostersForCleanup(
	ctx context.Context, userID, teamID, itemID string,
) []repositories.FeedItemReplyPoster {
	replyPosters, err := s.container.FeedItemReplyService().ListReplyPosters(ctx, userID, teamID, itemID)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "handleDeleteFeedItem",
			"user_id": userID,
			"team_id": teamID,
			"item_id": itemID,
			"error":   fmt.Sprintf("%+v", err),
		}).Warn("Failed to list reply posters for embedding cleanup (non-fatal); proceeding with delete")
		return nil
	}
	return replyPosters
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation helpers
// ─────────────────────────────────────────────────────────────────────────────

const (
	feedNameMaxLen          = 255
	feedDescriptionMaxLen   = 1000
	feedItemTitleMaxLen     = 255
	feedItemAssistantMaxLen = 30
	feedItemContentMaxSize  = 204800 // 200 KB
)

// validateCreateFeedRequest validates the create feed request fields.
func (s *Server) validateCreateFeedRequest(w http.ResponseWriter, req *models.CreateFeedRequest) bool {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErrorResponse(w, nil, "validation_error", "name is required", http.StatusBadRequest)
		return false
	}
	if len([]rune(name)) > feedNameMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("name cannot be longer than %d characters", feedNameMaxLen), http.StatusBadRequest)
		return false
	}
	if req.Description != nil && len([]rune(*req.Description)) > feedDescriptionMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("description cannot be longer than %d characters", feedDescriptionMaxLen), http.StatusBadRequest)
		return false
	}
	return true
}

// validateUpdateFeedRequest validates the update feed request fields.
func (s *Server) validateUpdateFeedRequest(w http.ResponseWriter, req *models.UpdateFeedRequest) bool {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeErrorResponse(w, nil, "validation_error", "name cannot be empty", http.StatusBadRequest)
			return false
		}
		if len([]rune(name)) > feedNameMaxLen {
			writeErrorResponse(w, nil, "validation_error",
				fmt.Sprintf("name cannot be longer than %d characters", feedNameMaxLen), http.StatusBadRequest)
			return false
		}
	}
	if req.Description != nil && len([]rune(*req.Description)) > feedDescriptionMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("description cannot be longer than %d characters", feedDescriptionMaxLen), http.StatusBadRequest)
		return false
	}
	return true
}

// validateCreateFeedItemRequest validates the create feed item request fields.
func (s *Server) validateCreateFeedItemRequest(w http.ResponseWriter, req *models.CreateFeedItemRequest) bool {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeErrorResponse(w, nil, "validation_error", "title is required", http.StatusBadRequest)
		return false
	}
	if len([]rune(title)) > feedItemTitleMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("title cannot be longer than %d characters", feedItemTitleMaxLen), http.StatusBadRequest)
		return false
	}

	if req.Content == "" {
		writeErrorResponse(w, nil, "validation_error", "content is required", http.StatusBadRequest)
		return false
	}
	if len(req.Content) > feedItemContentMaxSize {
		writeErrorResponse(w, nil, "validation_error",
			"content exceeds maximum size of 200 KB", http.StatusBadRequest)
		return false
	}

	assistant := strings.TrimSpace(req.AIAssistantName)
	if assistant == "" {
		writeErrorResponse(w, nil, "validation_error", "ai_assistant_name is required", http.StatusBadRequest)
		return false
	}
	if len([]rune(assistant)) > feedItemAssistantMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("ai_assistant_name cannot be longer than %d characters", feedItemAssistantMaxLen),
			http.StatusBadRequest)
		return false
	}

	if req.ProjectID != nil && *req.ProjectID != "" && !isValidUUID(*req.ProjectID) {
		writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
		return false
	}

	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Filter builder helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildFeedFilters constructs FeedFilters from the HTTP request query params.
func (s *Server) buildFeedFilters(r *http.Request, teamID string) services.FeedFilters {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	pagination := validatePaginationParams(r.URL.Query().Get("page"), limitStr)
	return services.FeedFilters{
		TeamID: teamID,
		Search: r.URL.Query().Get("search"),
		Page:   pagination.Page,
		Limit:  pagination.Limit,
	}
}

// parseFeedItemArchivedParam parses the ?archived query parameter.
// Returns (nil, nil) for "all", (*bool, nil) for "true"/"false"/"",
// and (nil, error) for any unrecognised value.
func parseFeedItemArchivedParam(val string) (*bool, error) {
	switch val {
	case "true":
		t := true
		return &t, nil
	case "false", "":
		// "false" or absent → show only active items
		f := false
		return &f, nil
	case "all":
		// nil means no filter — return all items regardless of archive state
		return nil, nil
	default:
		return nil, fmt.Errorf("archived must be one of: true, false, all")
	}
}

// buildFeedItemFilters constructs FeedItemFilters from the HTTP request query params.
// feedIDOverride, when non-empty, pins the FeedID filter (used for the per-feed items endpoint).
func (s *Server) buildFeedItemFilters(
	r *http.Request, teamID, feedIDOverride string,
) (services.FeedItemFilters, error) {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	pagination := validatePaginationParams(r.URL.Query().Get("page"), limitStr)

	filters := services.FeedItemFilters{
		TeamID: teamID,
		Page:   pagination.Page,
		Limit:  pagination.Limit,
	}

	// feed_id: prefer URL-path override, fall back to query param
	effectiveFeedID := feedIDOverride
	if effectiveFeedID == "" {
		if qFeedID := r.URL.Query().Get("feed_id"); qFeedID != "" {
			if !isValidUUID(qFeedID) {
				return services.FeedItemFilters{}, fmt.Errorf("feed_id must be a valid UUID")
			}
			effectiveFeedID = qFeedID
		}
	}
	if effectiveFeedID != "" {
		filters.FeedID = &effectiveFeedID
	}

	if pid := r.URL.Query().Get("project_id"); pid != "" {
		if !isValidUUID(pid) {
			return services.FeedItemFilters{}, fmt.Errorf("project_id must be a valid UUID")
		}
		filters.ProjectID = &pid
	}
	if aname := r.URL.Query().Get("ai_assistant_name"); aname != "" {
		filters.AIAssistantName = &aname
	}

	archived, err := parseFeedItemArchivedParam(r.URL.Query().Get("archived"))
	if err != nil {
		return services.FeedItemFilters{}, err
	}
	filters.Archived = archived

	return filters, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Error handling helpers
// ─────────────────────────────────────────────────────────────────────────────

// handleFeedServiceError maps service errors for Feed create/delete operations.
func (s *Server) handleFeedServiceError(w http.ResponseWriter, handler, userID, teamID string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
		"user_id": userID,
		"team_id": teamID,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Feed service error")

	msg := err.Error()
	switch {
	case strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate"):
		writeErrorResponse(w, nil, "conflict", "A feed with that name already exists in this team", http.StatusConflict)
	case strings.Contains(msg, "user is not a member of the specified team"):
		writeErrorResponse(w, nil, "forbidden", msg, http.StatusForbidden)
	case strings.Contains(msg, "not found"):
		writeErrorResponse(w, nil, "not_found", "Feed not found", http.StatusNotFound)
	default:
		writeErrorResponse(w, nil, "internal_error", "Failed to process feed request", http.StatusInternalServerError)
	}
}

// handleFeedGetError maps service errors for Feed get/update/delete operations.
func (s *Server) handleFeedGetError(w http.ResponseWriter, handler, userID, teamID, feedID string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Feed operation error")

	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"):
		writeErrorResponse(w, nil, "not_found", "Feed not found", http.StatusNotFound)
	case strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate"):
		writeErrorResponse(w, nil, "conflict", "A feed with that name already exists in this team", http.StatusConflict)
	case strings.Contains(msg, "user is not a member of the specified team"):
		writeErrorResponse(w, nil, "forbidden", msg, http.StatusForbidden)
	default:
		writeErrorResponse(w, nil, "internal_error", "Failed to process feed request", http.StatusInternalServerError)
	}
}

// handleFeedItemServiceError maps service errors for FeedItem create operations.
func (s *Server) handleFeedItemServiceError(w http.ResponseWriter, handler, userID, teamID, feedID string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
		"user_id": userID,
		"team_id": teamID,
		"feed_id": feedID,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Feed item service error")

	msg := err.Error()
	switch {
	case strings.Contains(msg, "user is not a member of the specified team"):
		writeErrorResponse(w, nil, "forbidden", msg, http.StatusForbidden)
	case strings.Contains(msg, "project not found"):
		writeErrorResponse(w, nil, "bad_request", "Project not found or does not belong to user", http.StatusBadRequest)
	case strings.Contains(msg, "project does not belong to the specified team"):
		writeErrorResponse(w, nil, "forbidden", "Project does not belong to the specified team", http.StatusForbidden)
	case strings.Contains(msg, "not found"):
		writeErrorResponse(w, nil, "not_found", "Feed or item not found", http.StatusNotFound)
	default:
		writeErrorResponse(w, nil, "internal_error", "Failed to process feed item request", http.StatusInternalServerError)
	}
}

// handleFeedItemGetError maps service errors for FeedItem get/archive/delete operations.
func (s *Server) handleFeedItemGetError(w http.ResponseWriter, handler, userID, teamID, itemID string, err error) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Feed item operation error")

	switch {
	case errors.Is(err, repositories.ErrFeedItemForbidden):
		writeErrorResponse(w, nil, "forbidden",
			"You do not have permission to perform this action on this feed item", http.StatusForbidden)
	case errors.Is(err, repositories.ErrFeedItemNotFound):
		writeErrorResponse(w, nil, "not_found", "Feed item not found", http.StatusNotFound)
	case strings.Contains(err.Error(), "user is not a member of the specified team"):
		writeErrorResponse(w, nil, "forbidden", err.Error(), http.StatusForbidden)
	case strings.Contains(err.Error(), "not found"):
		writeErrorResponse(w, nil, "not_found", "Feed item not found", http.StatusNotFound)
	default:
		writeErrorResponse(w, nil, "internal_error", "Failed to process feed item request", http.StatusInternalServerError)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource limit helpers
// ─────────────────────────────────────────────────────────────────────────────

// checkFeedResourceLimit checks if user has reached their feed resource limit.
// Returns true if the operation is allowed, false if the limit has been exceeded or an error occurred.
func (s *Server) checkFeedResourceLimit(w http.ResponseWriter, ctx context.Context, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, events.ResourceTypeFeed)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "checkFeedResourceLimit",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to check feed resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"handler":       "checkFeedResourceLimit",
			"user_id":       userID,
			"resource_type": events.ResourceTypeFeed,
		}).Warn("User has reached their feed limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of feeds allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

// checkFeedItemResourceLimit checks if user has reached their feed item resource limit.
// Returns true if the operation is allowed, false if the limit has been exceeded or an error occurred.
func (s *Server) checkFeedItemResourceLimit(w http.ResponseWriter, ctx context.Context, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, events.ResourceTypeFeedItem)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service": "vibexp-api",
			"handler": "checkFeedItemResourceLimit",
			"user_id": userID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to check feed item resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.WithFields(logrus.Fields{
			"service":       "vibexp-api",
			"handler":       "checkFeedItemResourceLimit",
			"user_id":       userID,
			"resource_type": events.ResourceTypeFeedItem,
		}).Warn("User has reached their feed item limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of feed items allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Feed item reply handlers
// ─────────────────────────────────────────────────────────────────────────────

const (
	feedItemReplyContentMaxLen   = 10000
	feedItemReplyAssistantMaxLen = 30
)

func (s *Server) handleCreateFeedItemReply(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleCreateFeedItemReply",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("Create feed item reply request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	var req models.CreateFeedItemReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateFeedItemReplyRequest(w, &req) {
		return
	}

	if !s.checkFeedItemResourceLimit(w, r.Context(), userID) {
		return
	}

	reply, err := s.container.FeedItemReplyService().CreateReply(r.Context(), userID, teamID, itemID, &req)
	if err != nil {
		s.handleFeedItemReplyServiceError(w, "handleCreateFeedItemReply", userID, teamID, itemID, err)
		return
	}

	writeCreated(w, reply, s.logger)
}

func (s *Server) handleListFeedItemReplies(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	itemID := chi.URLParam(r, "item_id")

	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": "handleListFeedItemReplies",
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
	}).Info("List feed item replies request received")

	if !isValidUUID(itemID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid item_id format", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	pagination := validatePaginationParams(r.URL.Query().Get("page"), limitStr)

	response, err := s.container.FeedItemReplyService().ListReplies(
		r.Context(), userID, teamID, itemID, pagination.Page, pagination.Limit,
	)
	if err != nil {
		s.handleFeedItemReplyServiceError(w, "handleListFeedItemReplies", userID, teamID, itemID, err)
		return
	}

	writeOK(w, response, s.logger)
}

// validateCreateFeedItemReplyRequest validates the create feed item reply request fields.
func (s *Server) validateCreateFeedItemReplyRequest(
	w http.ResponseWriter, req *models.CreateFeedItemReplyRequest,
) bool {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeErrorResponse(w, nil, "validation_error", "content is required", http.StatusBadRequest)
		return false
	}
	if len([]rune(content)) > feedItemReplyContentMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("content cannot be longer than %d characters", feedItemReplyContentMaxLen),
			http.StatusBadRequest)
		return false
	}
	req.Content = content
	if req.AIAssistantName != nil && len([]rune(*req.AIAssistantName)) > feedItemReplyAssistantMaxLen {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("ai_assistant_name cannot be longer than %d characters", feedItemReplyAssistantMaxLen),
			http.StatusBadRequest)
		return false
	}
	return true
}

// handleFeedItemReplyServiceError maps service errors for FeedItemReply operations.
func (s *Server) handleFeedItemReplyServiceError(
	w http.ResponseWriter, handler, userID, teamID, itemID string, err error,
) {
	s.logger.WithFields(logrus.Fields{
		"service": "vibexp-api",
		"handler": handler,
		"user_id": userID,
		"team_id": teamID,
		"item_id": itemID,
		"error":   fmt.Sprintf("%+v", err),
	}).Error("Feed item reply service error")

	msg := err.Error()
	switch {
	case strings.Contains(msg, "feed item is archived"):
		writeErrorResponse(w, nil, "unprocessable_entity",
			"Cannot reply to an archived feed item", http.StatusUnprocessableEntity)
	case strings.Contains(msg, "user is not a member of the specified team"):
		writeErrorResponse(w, nil, "forbidden", msg, http.StatusForbidden)
	case strings.Contains(msg, "not found"):
		writeErrorResponse(w, nil, "not_found", "Feed item not found", http.StatusNotFound)
	default:
		writeErrorResponse(w, nil, "internal_error",
			"Failed to process feed item reply request", http.StatusInternalServerError)
	}
}
