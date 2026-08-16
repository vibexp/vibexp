package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// vibexp_io_list_teams_and_projects — the single workspace-discovery tool (#814,
// epic #811).
//
// It replaces the pair it supersedes because the pair structurally could not
// answer the question agents actually ask. `list_teams` cannot mention projects;
// `list_projects` cannot be called before the team is known. An agent holding
// only a project name had to list teams, then list projects per team, paginating
// each — roughly 25 calls on a six-team account — and agents that truncate that
// resolve the wrong workspace. Hence one tool, an explicit name (an agent
// hunting a project never opens something called `list_teams`), and lean nested
// payloads.

const (
	// workspaceDefaultLimit is the page size when the caller does not ask.
	workspaceDefaultLimit = 10
	// workspaceMaxLimit caps the page size. This feeds an agent's context window;
	// a bigger page is how the truncation this tool exists to prevent starts.
	workspaceMaxLimit = 25
	// workspaceDescriptionLimit truncates project descriptions, counted in RUNES
	// so a multi-byte character is never cut in half.
	workspaceDescriptionLimit = 160
	// workspaceTeamLookupLimit bounds the team lookup used to attach names to
	// project hits. It is deliberately generous relative to any real membership
	// and, unlike the loop it replaces, it terminates.
	workspaceTeamLookupLimit = 100
	// workspaceMaxSearchDepth bounds how deep search paging can reach. One ladder
	// run decides which pass answers a query, so pages must be sliced out of a
	// SINGLE run — re-running per page could silently switch passes between page
	// 1 and page 2 and return a differently-ranked result set.
	workspaceMaxSearchDepth = 100
)

// Scope values. Anything else falls back to scopeBoth rather than erroring: an
// unrecognised scope is a model typo, and answering the broader question beats
// failing the call.
const (
	scopeTeams    = "teams"
	scopeProjects = "projects"
	scopeBoth     = "both"
)

// listTeamsAndProjectsToolDescription is the agent-facing description.
//
//nolint:lll // verbatim agent-facing description; wrapping it changes the text the model reads
const listTeamsAndProjectsToolDescription = "Discover the workspace: find teams and projects, or list them. " +
	"Call this FIRST to obtain a team_id (UUID or slug) or a project slug before using any other tool. " +
	"With no arguments it returns the teams you belong to with their project counts. " +
	"With `query` it searches team AND project names, slugs, descriptions and project git URLs " +
	"across ALL your teams at once, returning matching teams with their matching projects nested and ranked — " +
	"use this when you know a project or repository name but not which team holds it. " +
	"With `team_id` and no query it lists that team's projects. " +
	"Typo-tolerant: a single mistyped character still matches. " +
	"Replaces vibexp_io_list_teams and vibexp_io_list_projects."

// ListTeamsAndProjectsParams defines the parameters of the merged discovery tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListTeamsAndProjectsParams struct {
	Query  string `json:"query,omitempty"   jsonschema:"Search text. Matches team name/slug/description and project name/slug/description/git_url across every team you belong to. Typo-tolerant."`
	TeamID string `json:"team_id,omitempty" jsonschema:"Optional team UUID or slug. Narrows the result to one team; without it every team you belong to is searched."`
	Scope  string `json:"scope,omitempty"   jsonschema:"What to return: teams, projects, or both (default both)."`
	Page   int    `json:"page,omitempty"    jsonschema:"Page number (default: 1)."`
	Limit  int    `json:"limit,omitempty"   jsonschema:"Items per page (default: 10, max: 25)."`
}

// workspaceProject is the LEAN project shape this tool returns. It is
// deliberately not models.Project: that carries user_id, team_id (redundant when
// nested under its team), version, homepage and both timestamps — measured at
// ≈3.5 KB for ten projects, none of which an agent choosing a workspace needs.
// TestWorkspaceProjectFieldSet pins these fields so the fat struct cannot leak
// back in through a later refactor.
type workspaceProject struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

// workspaceTeam is a team plus, when the call asked for them, its projects.
// `projects` is omitempty so the no-argument orientation call is strictly
// smaller than the list_teams response it replaces.
type workspaceTeam struct {
	UUID         string             `json:"uuid"`
	Name         string             `json:"name"`
	Slug         string             `json:"slug"`
	ProjectCount int                `json:"project_count"`
	Projects     []workspaceProject `json:"projects,omitempty"`
}

// listTeamsAndProjectsResponse is the tool's response shape.
type listTeamsAndProjectsResponse struct {
	Teams []workspaceTeam `json:"teams"`
}

// workspaceQuery is the normalised, validated form of the tool's parameters.
type workspaceQuery struct {
	query  string
	teamID string
	scope  string
	page   int
	limit  int
}

// normalizeWorkspaceParams applies defaults and caps. Invalid input is corrected
// rather than rejected — every field here is model-supplied, and a discovery
// call that fails on a typo'd scope teaches the agent to stop using the tool.
func normalizeWorkspaceParams(params *ListTeamsAndProjectsParams) workspaceQuery {
	q := workspaceQuery{
		query:  params.Query,
		teamID: params.TeamID,
		scope:  params.Scope,
		page:   params.Page,
		limit:  params.Limit,
	}

	switch q.scope {
	case scopeTeams, scopeProjects, scopeBoth:
	default:
		q.scope = scopeBoth
	}
	if q.page <= 0 {
		q.page = 1
	}
	if q.limit <= 0 {
		q.limit = workspaceDefaultLimit
	}
	if q.limit > workspaceMaxLimit {
		q.limit = workspaceMaxLimit
	}

	return q
}

// wantsTeams / wantsProjects read the scope.
func (q workspaceQuery) wantsTeams() bool    { return q.scope != scopeProjects }
func (q workspaceQuery) wantsProjects() bool { return q.scope != scopeTeams }

// truncateRunes shortens s to at most limit RUNES, appending an ellipsis when it
// cut. Counting runes rather than bytes is what keeps a multi-byte character
// from being split into invalid UTF-8 halfway through a description.
func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

// listTeamsAndProjects implements vibexp_io_list_teams_and_projects.
//
// Three behaviours share one handler because they are one question asked with
// varying amounts of knowledge:
//   - no query, no team    → the teams you belong to (orientation)
//   - query                → ranked cross-team search, projects nested
//   - team_id, no query    → that team's projects
func (s *Server) listTeamsAndProjects(
	ctx context.Context, _ *mcp.CallToolRequest, params *ListTeamsAndProjectsParams, userID string,
) (*mcp.CallToolResult, any, error) {
	q := normalizeWorkspaceParams(params)

	// resolveTeam is mandatory-first for any team-scoped input, and after #812 it
	// is a single query. It also supplies the generic access-denied text, so an
	// unknown team and a team the caller cannot see stay indistinguishable.
	if q.teamID != "" {
		resolved, errResult := s.resolveTeam(ctx, userID, q.teamID)
		if errResult != nil {
			return errResult, nil, nil
		}
		q.teamID = resolved
	}

	var (
		teams []workspaceTeam
		err   error
	)
	if q.query != "" {
		teams, err = s.searchWorkspace(ctx, userID, q)
	} else {
		teams, err = s.listWorkspace(ctx, userID, q)
	}
	if err != nil {
		slog.With(
			"tool", "vibexp_io_list_teams_and_projects",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list teams and projects via MCP")
		return mcpTextError("Failed to list teams and projects. Please try again later."), nil, nil
	}

	result := &listTeamsAndProjectsResponse{Teams: teams}
	jsonData, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", marshalErr)
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		StructuredContent: result,
	}, result, nil
}

// listWorkspace answers the no-query calls: the caller's teams (orientation), or
// one team's projects when team_id was supplied.
func (s *Server) listWorkspace(ctx context.Context, userID string, q workspaceQuery) ([]workspaceTeam, error) {
	if q.teamID != "" {
		return s.listOneTeamsProjects(ctx, userID, q)
	}

	offset := (q.page - 1) * q.limit
	teamRows, _, err := s.container.TeamRepository().ListByUserID(ctx, userID, q.limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	teams, err := s.annotateTeams(ctx, teamRows)
	if err != nil {
		return nil, err
	}

	// scope=projects with no team and no query has nothing to narrow to, so the
	// teams still come back — with their counts — rather than an empty array that
	// reads as "you have no workspaces".
	return teams, nil
}

// listOneTeamsProjects is the behaviour vibexp_io_list_projects used to serve.
func (s *Server) listOneTeamsProjects(
	ctx context.Context, userID string, q workspaceQuery,
) ([]workspaceTeam, error) {
	team, err := s.container.TeamRepository().GetByID(ctx, q.teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to load resolved team: %w", err)
	}

	entry := workspaceTeam{UUID: team.ID, Name: team.Name, Slug: team.Slug}

	counts, err := s.container.ProjectRepository().CountsByTeamIDs(ctx, []string{team.ID})
	if err != nil {
		return nil, fmt.Errorf("failed to count projects: %w", err)
	}
	entry.ProjectCount = counts[team.ID]

	if q.wantsProjects() {
		listResp, listErr := s.container.ProjectService().ListProjects(userID, services.ProjectFilters{
			TeamID: q.teamID,
			Page:   q.page,
			Limit:  q.limit,
		})
		if listErr != nil {
			return nil, fmt.Errorf("failed to list projects: %w", listErr)
		}
		entry.Projects = leanProjects(listResp.Projects)
	}

	return []workspaceTeam{entry}, nil
}

// searchWorkspace answers a query: ranked teams and, nested under their teams,
// ranked projects — across every team the caller belongs to unless team_id
// narrows it.
//
// Paging slices a SINGLE ladder run rather than re-running per page, because the
// ladder picks one pass per run: re-running with an offset could answer page 1
// from the exact pass and page 2 from the trigram pass, silently mixing two
// ranking scales. The depth bound is the price, and it is stated in the code
// rather than hidden.
func (s *Server) searchWorkspace(ctx context.Context, userID string, q workspaceQuery) ([]workspaceTeam, error) {
	depth := q.page * q.limit
	if depth > workspaceMaxSearchDepth {
		depth = workspaceMaxSearchDepth
	}
	offset := (q.page - 1) * q.limit

	byTeam := map[string][]workspaceProject{}
	if q.wantsProjects() {
		hits, err := s.container.ProjectRepository().SearchProjects(ctx, userID, repositories.ProjectSearchFilters{
			Query:  q.query,
			TeamID: q.teamID,
			Limit:  depth,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to search projects: %w", err)
		}
		for _, hit := range pageOf(hits, offset, q.limit) {
			byTeam[hit.TeamID] = append(byTeam[hit.TeamID], leanProject(hit.Project, hit.Score))
		}
	}

	var matchedTeams []models.Team
	if q.wantsTeams() {
		hits, err := s.container.TeamRepository().SearchTeams(ctx, userID, q.query, depth)
		if err != nil {
			return nil, fmt.Errorf("failed to search teams: %w", err)
		}
		for _, hit := range pageOf(hits, offset, q.limit) {
			if q.teamID != "" && hit.ID != q.teamID {
				continue
			}
			matchedTeams = append(matchedTeams, hit.Team)
		}
	}

	return s.assembleSearchResult(ctx, userID, matchedTeams, byTeam)
}

// assembleSearchResult merges the teams that matched the query with the teams
// that merely HOLD a matching project, so a project hit is never orphaned under
// a team the response cannot name.
func (s *Server) assembleSearchResult(
	ctx context.Context, userID string, matched []models.Team, byTeam map[string][]workspaceProject,
) ([]workspaceTeam, error) {
	ordered := make([]models.Team, 0, len(matched)+len(byTeam))
	seen := make(map[string]bool, len(matched)+len(byTeam))
	for _, team := range matched {
		if !seen[team.ID] {
			seen[team.ID] = true
			ordered = append(ordered, team)
		}
	}

	// Teams that hold a matching project but did not match themselves still need
	// a name and slug. One bounded lookup of the caller's own teams covers every
	// such team, instead of a GetByID per team id.
	if missing := missingTeamIDs(byTeam, seen); len(missing) > 0 {
		owned, _, err := s.container.TeamRepository().ListByUserID(ctx, userID, workspaceTeamLookupLimit, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to load teams for project hits: %w", err)
		}
		for i := range owned {
			if missing[owned[i].ID] {
				seen[owned[i].ID] = true
				ordered = append(ordered, owned[i])
			}
		}
	}

	teams, err := s.annotateTeams(ctx, ordered)
	if err != nil {
		return nil, err
	}
	for i := range teams {
		teams[i].Projects = byTeam[teams[i].UUID]
	}

	return teams, nil
}

// missingTeamIDs returns the team ids that hold matching projects but are not
// already accounted for.
func missingTeamIDs(byTeam map[string][]workspaceProject, seen map[string]bool) map[string]bool {
	missing := map[string]bool{}
	for teamID := range byTeam {
		if !seen[teamID] {
			missing[teamID] = true
		}
	}
	return missing
}

// annotateTeams projects team rows into the response shape and attaches project
// counts in ONE grouped query.
func (s *Server) annotateTeams(ctx context.Context, rows []models.Team) ([]workspaceTeam, error) {
	teams := make([]workspaceTeam, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for i := range rows {
		teams = append(teams, workspaceTeam{
			UUID: rows[i].ID,
			Name: rows[i].Name,
			Slug: rows[i].Slug,
		})
		ids = append(ids, rows[i].ID)
	}

	counts, err := s.container.ProjectRepository().CountsByTeamIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to count projects: %w", err)
	}
	for i := range teams {
		teams[i].ProjectCount = counts[teams[i].UUID]
	}

	return teams, nil
}

// pageOf slices one page out of an already-ranked result set.
func pageOf[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

// leanProject projects one project into the narrow response shape.
func leanProject(p models.Project, score float64) workspaceProject {
	return workspaceProject{
		ID:          p.ID,
		Slug:        p.Slug,
		Name:        p.Name,
		Description: truncateRunes(p.Description, workspaceDescriptionLimit),
		Score:       score,
	}
}

// leanProjects projects a listing into the narrow shape. A listing carries no
// relevance score, so score is omitted (it is `omitempty`) rather than reported
// as a meaningless 0 that an agent might compare against a real one.
func leanProjects(projects []models.ProjectResponse) []workspaceProject {
	lean := make([]workspaceProject, 0, len(projects))
	for i := range projects {
		lean = append(lean, leanProject(projects[i].Project, 0))
	}
	return lean
}
