-- Per-team search ranking overrides (#488, epic #487).
--
-- Search ranking is currently frozen into a process-lifetime singleton, built
-- at startup from the `search:` block of config.yaml and therefore identical
-- for every team on the instance. This table is the storage foundation for
-- letting a team own its own ranking profile.
--
-- Whole-row override, NOT per-field NULL inheritance: a team either owns its
-- complete profile or inherits the instance defaults entirely. That keeps the
-- mental model coherent — a team's relevance weight is never blended with the
-- instance's created weight. Consequently every profile column is NOT NULL.

CREATE TABLE team_search_settings (
    -- team_id is the PRIMARY KEY (not a surrogate id + UNIQUE, as
    -- user_preferences does): the one-row-per-team singleton is then enforced
    -- by the schema itself rather than by application code.
    team_id                 uuid PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,

    recency_ranking_enabled boolean          NOT NULL,
    rank_weight_relevance   double precision NOT NULL,
    rank_weight_created     double precision NOT NULL,
    rank_weight_updated     double precision NOT NULL,
    rank_half_life_days     double precision NOT NULL,

    created_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version                 bigint      NOT NULL DEFAULT 1,

    -- These CHECKs deliberately mirror validateSearchRankingConfig
    -- (backend/internal/config/config.go) bound for bound, so a team can never
    -- persist a profile the operator themself could not configure. If either
    -- side's bounds change, change both.
    CONSTRAINT team_search_settings_weights_nonneg CHECK (
        rank_weight_relevance >= 0 AND rank_weight_created >= 0 AND rank_weight_updated >= 0),
    CONSTRAINT team_search_settings_weights_nonzero CHECK (
        rank_weight_relevance + rank_weight_created + rank_weight_updated > 0),
    CONSTRAINT team_search_settings_half_life CHECK (
        rank_half_life_days > 0 AND rank_half_life_days <= 36500)
);

-- No index beyond the primary key: the only query this table ever serves is
-- `WHERE team_id = $1`, which the PK already covers.

COMMENT ON TABLE team_search_settings IS
    'Optional per-team override of the instance search ranking defaults (config.yaml `search:`). Row absent = inherit instance defaults. rank_candidate_cap is deliberately absent: it stays instance-only.';

COMMENT ON COLUMN team_search_settings.rank_half_life_days IS
    'Freshness half-life in days, capped at 36500 (100 years) to keep the days->time.Duration conversion clear of int64 nanosecond overflow.';
