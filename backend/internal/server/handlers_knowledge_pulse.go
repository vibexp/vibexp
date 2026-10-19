package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	knowledgepulsegen "github.com/vibexp/vibexp/internal/server/gen/knowledgepulse"
	"github.com/vibexp/vibexp/internal/services"
)

// Knowledge Pulse: one read-only, polled endpoint. The tenancy
// middleware is the whole authorization story, as for the other team
// analytics reads.
//
// Every required array is built with make(..., 0, n), so it serializes as []
// and never null: generated strict-server types cannot use models.JSONArray,
// which is why these schemas sit in adHocRequiredArrayAllowlist.

const knowledgePulseMsgInternalError = "Failed to build the knowledge pulse"

type knowledgePulseStrictServer struct {
	s *Server
}

var _ knowledgepulsegen.StrictServerInterface = (*knowledgePulseStrictServer)(nil)

// setupKnowledgePulseRoutes mounts the generated strict-server handler under a
// team-validated group, mirroring setupFreshnessRoutes.
func (s *Server) setupKnowledgePulseRoutes(r chi.Router) {
	strict := knowledgepulsegen.NewStrictHandlerWithOptions(
		&knowledgePulseStrictServer{s: s},
		nil,
		knowledgepulsegen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.knowledgePulseBindErrorHandler,
			ResponseErrorHandlerFunc: s.knowledgePulseResponseErrorHandler,
		},
	)
	r.Group(func(gr chi.Router) {
		gr.Use(s.teamValidationMiddleware()) // Validate team_id from URL and team access
		knowledgepulsegen.HandlerWithOptions(strict, knowledgepulsegen.ChiServerOptions{
			BaseRouter:       gr,
			ErrorHandlerFunc: s.knowledgePulseBindErrorHandler,
		})
	})
}

// GetKnowledgePulse returns the merged events after the cursor.
func (ks *knowledgePulseStrictServer) GetKnowledgePulse(
	ctx context.Context, request knowledgepulsegen.GetKnowledgePulseRequestObject,
) (knowledgepulsegen.GetKnowledgePulseResponseObject, error) {
	query := services.KnowledgePulseQuery{}
	if request.Params.Since != nil && *request.Params.Since != "" {
		since, err := time.Parse(time.RFC3339Nano, *request.Params.Since)
		if err != nil {
			return nil, apierrors.NewBadRequestError("since must be an RFC 3339 timestamp")
		}
		query.Since = &since
	}
	if request.Params.Limit != nil {
		query.Limit = *request.Params.Limit
	}
	if request.Params.Snapshot != nil {
		query.IncludeSnapshot = *request.Params.Snapshot
	}

	result, err := ks.s.container.KnowledgePulseService().GetPulse(ctx, request.TeamId.String(), query)
	if err != nil {
		ks.s.logger.With("error", err, "operation", "GetKnowledgePulse").Error("Knowledge pulse request failed")
		return nil, apierrors.NewInternalError(knowledgePulseMsgInternalError)
	}
	return knowledgepulsegen.GetKnowledgePulse200JSONResponse(toGenKnowledgePulse(result)), nil
}

func toGenKnowledgePulse(result *services.KnowledgePulseResult) knowledgepulsegen.KnowledgePulseResponse {
	events := make([]knowledgepulsegen.KnowledgePulseEvent, 0, len(result.Events))
	for _, ev := range result.Events {
		events = append(events, toGenPulseEvent(ev))
	}
	resp := knowledgepulsegen.KnowledgePulseResponse{
		Cursor: result.Cursor.UTC().Format(time.RFC3339Nano),
		Events: events,
		StatsToday: knowledgepulsegen.KnowledgePulseStats{
			Reads:            result.Stats.Reads,
			Writes:           result.Stats.Writes,
			Links:            result.Stats.Links,
			CompoundingScore: result.Stats.CompoundingScore,
		},
	}
	if result.Snapshot != nil {
		snap := toGenPulseSnapshot(result.Snapshot)
		resp.Snapshot = &snap
	}
	return resp
}

func toGenPulseEvent(ev services.PulseEvent) knowledgepulsegen.KnowledgePulseEvent {
	out := knowledgepulsegen.KnowledgePulseEvent{
		Id:   ev.ID,
		Kind: knowledgepulsegen.KnowledgePulseEventKind(ev.Kind),
		At:   ev.At.UTC(),
		Actor: knowledgepulsegen.KnowledgePulseActor{
			Type:   knowledgepulsegen.KnowledgePulseActorType(ev.Actor.Type),
			Id:     ev.Actor.ID,
			Name:   ev.Actor.Name,
			Source: knowledgepulsegen.KnowledgePulseSource(ev.Actor.Source),
		},
		Resource: knowledgepulsegen.KnowledgePulseResourceRef{
			Type:      knowledgepulsegen.KnowledgePulseResourceType(ev.Resource.Type),
			Id:        ev.Resource.ID,
			Title:     ev.Resource.Title,
			Slug:      optionalString(ev.Resource.Slug),
			ProjectId: optionalString(ev.Resource.ProjectID),
		},
	}
	if ev.Action == "" && ev.RelationType == "" && ev.To == nil {
		return out
	}
	detail := &knowledgepulsegen.KnowledgePulseEventDetail{RelationType: optionalString(ev.RelationType)}
	if ev.Action != "" {
		action := knowledgepulsegen.KnowledgePulseEventDetailAction(ev.Action)
		detail.Action = &action
	}
	if ev.To != nil {
		detail.To = &knowledgepulsegen.KnowledgePulseRelatedRef{
			Type:  knowledgepulsegen.KnowledgePulseResourceType(ev.To.Type),
			Id:    ev.To.ID,
			Title: optionalString(ev.To.Title),
			Slug:  optionalString(ev.To.Slug),
		}
	}
	out.Detail = detail
	return out
}

func toGenPulseSnapshot(snap *services.PulseSnapshot) knowledgepulsegen.KnowledgePulseSnapshot {
	nodes := make([]knowledgepulsegen.KnowledgePulseNode, 0, len(snap.Nodes))
	for _, n := range snap.Nodes {
		nodes = append(nodes, toGenPulseNode(n))
	}
	edges := make([]knowledgepulsegen.KnowledgePulseEdge, 0, len(snap.Edges))
	for _, e := range snap.Edges {
		edges = append(edges, knowledgepulsegen.KnowledgePulseEdge{
			From: e.FromID, To: e.ToID, RelationType: e.RelationType,
		})
	}
	actors := make([]knowledgepulsegen.KnowledgePulseSnapshotActor, 0, len(snap.Actors))
	for _, a := range snap.Actors {
		actors = append(actors, knowledgepulsegen.KnowledgePulseSnapshotActor{
			Id: a.ID, Name: a.Name, Type: knowledgepulsegen.KnowledgePulseActorType(a.Type),
		})
	}
	return knowledgepulsegen.KnowledgePulseSnapshot{Nodes: nodes, Edges: edges, Actors: actors}
}

func toGenPulseNode(n models.PulseNode) knowledgepulsegen.KnowledgePulseNode {
	return knowledgepulsegen.KnowledgePulseNode{
		Type:      knowledgepulsegen.KnowledgePulseResourceType(n.Resource.Type),
		Id:        n.Resource.ID,
		Title:     n.Resource.Title,
		Slug:      optionalString(n.Resource.Slug),
		ProjectId: optionalString(n.Resource.ProjectID),
		Degree:    n.Degree,
	}
}

// knowledgePulseBindErrorHandler turns a parameter-binding failure into a 400.
func (s *Server) knowledgePulseBindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	msg := err.Error()
	var invalidParam *knowledgepulsegen.InvalidParamFormatError
	if errors.As(err, &invalidParam) {
		msg = fmt.Sprintf("%s is not in the expected format", invalidParam.ParamName)
	}
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(msg))
}

// knowledgePulseResponseErrorHandler writes the API error a handler returned.
func (s *Server) knowledgePulseResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Unhandled knowledge pulse handler error")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(knowledgePulseMsgInternalError))
}
