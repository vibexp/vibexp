-- Instance-level search ranking and AI summary defaults (#1197, epic #1196).
--
-- Both have only ever lived in config.yaml (`search:` and `ai_summary:`),
-- captured once at wire time, so an instance admin could not change them
-- without file access and a restart -- and the published Docker image exposes
-- no `search:` knobs at all. These two tables are the storage that lets the
-- instance defaults be edited at runtime.
--
-- They mirror team_search_settings (011_consolidated) and
-- team_ai_summary_settings (017) with a singleton key in place of team_id, the
-- same shape instance_email_provider (020) uses. A team row cannot hold an
-- instance value: a NULL team_id would break the tenancy-only repository
-- invariant (epic decision 6).
--
-- No row means "use the built-in defaults", so DELETE is the reset (epic
-- decision 7). Every CHECK below mirrors a Go constant -- models.MaxSearchRank*,
-- config.MaxAISummaryTopN, config.MaxAISummaryOutputTokens and
-- models.AISummaryStyles -- and a change on either side must change both. The
-- CHECKs are the storage backstop; the services validate first.
--
-- Storage only: nothing reads these tables until the resolvers land (#1198,
-- #1199), so existing behaviour is unchanged.

CREATE TABLE instance_search_settings (
    -- Singleton key. It can only ever hold `true` (CHECK) and is the primary
    -- key, so a second row is a unique violation and no code path can create
    -- one.
    id                      boolean PRIMARY KEY DEFAULT true CHECK (id),
    recency_ranking_enabled boolean          NOT NULL,
    rank_weight_relevance   double precision NOT NULL,
    rank_weight_created     double precision NOT NULL,
    rank_weight_updated     double precision NOT NULL,
    rank_half_life_days     double precision NOT NULL,
    -- Instance-only: the re-rank candidate pool sizes the work one search asks
    -- of the database, so no team row carries it.
    rank_candidate_cap      integer          NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. NULL for the boot-time config.yaml import (no actor), and
    -- SET NULL on user deletion: removing an admin must not reset the
    -- instance's defaults.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1,
    CONSTRAINT instance_search_settings_weights_nonneg CHECK (
        rank_weight_relevance >= 0 AND rank_weight_created >= 0 AND rank_weight_updated >= 0),
    CONSTRAINT instance_search_settings_weights_nonzero CHECK (
        rank_weight_relevance + rank_weight_created + rank_weight_updated > 0),
    -- Mirrors models.MaxSearchRankHalfLifeDays.
    CONSTRAINT instance_search_settings_half_life CHECK (
        rank_half_life_days > 0 AND rank_half_life_days <= 36500),
    -- Mirrors models.MaxSearchRankCandidateCap.
    CONSTRAINT instance_search_settings_candidate_cap CHECK (
        rank_candidate_cap BETWEEN 1 AND 5000)
);

COMMENT ON TABLE instance_search_settings IS
    'Instance-level search ranking defaults. At most one row (singleton key id = true); no row = built-in defaults, so DELETE is the reset.';
COMMENT ON COLUMN instance_search_settings.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_search_settings.rank_half_life_days IS
    'Recency half-life in days, (0, 36500]. The bound mirrors models.MaxSearchRankHalfLifeDays; change both together.';
COMMENT ON COLUMN instance_search_settings.rank_candidate_cap IS
    'Instance-only re-rank candidate pool, [1, 5000]. The bound mirrors models.MaxSearchRankCandidateCap; change both together.';
COMMENT ON COLUMN instance_search_settings.updated_by IS
    'Last editor. NULL for the boot-time config.yaml import, and SET NULL when that user is deleted.';

CREATE TABLE instance_ai_summary_settings (
    id                  boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- The team-inheritable defaults (the columns team_ai_summary_settings
    -- overrides). There are deliberately NO max_top_n /
    -- max_output_tokens_ceiling columns (epic decision 3): the hard limits are
    -- the constants the CHECKs below mirror.
    enabled             boolean NOT NULL,
    -- Mirrors config.MaxAISummaryTopN.
    top_n               integer NOT NULL CHECK (top_n BETWEEN 1 AND 10),
    -- Mirrors models.AISummaryStyles.
    style               character varying(20) NOT NULL
                            CHECK (style IN ('concise', 'balanced', 'detailed')),
    -- Mirrors config.MaxAISummaryOutputTokens. The team table checks only
    -- > 0; the instance row pins the hard limit (epic decision 3).
    max_output_tokens   integer NOT NULL CHECK (max_output_tokens BETWEEN 1 AND 32768),
    -- Instance-only budgets: they size the work one request may ask of the
    -- operator's model and hardware, so a team cannot raise them.
    per_document_chars  integer NOT NULL CHECK (per_document_chars >= 1),
    total_context_chars integer NOT NULL,
    request_timeout_ms  integer NOT NULL CHECK (request_timeout_ms >= 1),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
    version    bigint NOT NULL DEFAULT 1,
    CONSTRAINT instance_ai_summary_settings_context_budget CHECK (
        total_context_chars >= per_document_chars)
);

COMMENT ON TABLE instance_ai_summary_settings IS
    'Instance-level AI summary defaults and budgets. At most one row (singleton key id = true); no row = built-in defaults, so DELETE is the reset.';
COMMENT ON COLUMN instance_ai_summary_settings.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_ai_summary_settings.top_n IS
    'Default documents per summary, [1, 10]. The bound mirrors config.MaxAISummaryTopN; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.style IS
    'Default summary style. The set mirrors models.AISummaryStyles; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.max_output_tokens IS
    'Default output token budget, [1, 32768]. The bound mirrors config.MaxAISummaryOutputTokens; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.total_context_chars IS
    'Instance-only total context budget; never smaller than per_document_chars.';
COMMENT ON COLUMN instance_ai_summary_settings.request_timeout_ms IS
    'Instance-only summary request timeout, in milliseconds.';
COMMENT ON COLUMN instance_ai_summary_settings.updated_by IS
    'Last editor. NULL for the boot-time config.yaml import, and SET NULL when that user is deleted.';
