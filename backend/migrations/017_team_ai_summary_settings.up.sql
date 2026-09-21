-- Per-team AI summary settings (#1071, epic #1068).
--
-- AI Summary is configured on two levels: the operator owns the context budgets
-- that have to match the deployment's hardware and model (the `ai_summary:`
-- block of config.yaml), and a team owns the choices that are editorial rather
-- than operational -- whether the feature is on, how many documents to feed it,
-- how long and how terse the answer should be, and which of the team's model
-- providers to use. This table is the storage for that second half.
--
-- Whole-row override, NOT per-field NULL inheritance: a team either owns its
-- complete profile or inherits the instance defaults entirely, exactly as
-- team_search_settings does (#488). Blending a team's top_n with the instance's
-- style would make "what is in effect" unanswerable without knowing which
-- columns happened to be set. Consequently every profile column is NOT NULL --
-- model_provider_id is the sole exception, and its NULL is a VALUE ("use the
-- team's default provider"), not an absence.

CREATE TABLE team_ai_summary_settings (
    -- team_id is the PRIMARY KEY (mirroring team_search_settings): the
    -- one-row-per-team singleton is enforced by the schema rather than by
    -- application code.
    team_id           uuid PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,

    -- Explicit on/off, independent of whether the team has a usable provider.
    -- A team that has configured no model provider still has a representable
    -- "we want this on" preference, and an admin can switch the feature off
    -- without tearing down their providers.
    enabled           boolean NOT NULL,

    -- ON DELETE SET NULL, deliberately: deleting a model provider must not
    -- delete the team's whole summary profile (CASCADE) and must not leave a
    -- dangling id (no FK). NULL means "fall back to the team's default
    -- provider", which is also the state a team starts in.
    model_provider_id uuid REFERENCES model_providers(id) ON DELETE SET NULL,

    -- top_n's upper bound mirrors config.MaxAISummaryTopN. The CHECK is the
    -- absolute ceiling for the whole deployment; `ai_summary.max_top_n` tunes
    -- the per-team limit DOWNWARD inside it and is validated against the same
    -- constant at startup, so the service can never accept a value this
    -- constraint would reject. If either side's bound changes, change both.
    top_n             integer NOT NULL CHECK (top_n >= 1 AND top_n <= 10),

    -- The style vocabulary is closed and shared with config validation and the
    -- summary generator (#1073). A CHECK rather than an enum type: adding a
    -- style should be one ALTER, not a type migration.
    style             character varying(20) NOT NULL
                          CHECK (style IN ('concise', 'balanced', 'detailed')),

    max_output_tokens integer NOT NULL CHECK (max_output_tokens > 0),

    created_at        timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version           bigint      NOT NULL DEFAULT 1
);

-- No index beyond the primary key: the only query this table serves is
-- `WHERE team_id = $1`, which the PK already covers. model_provider_id is not
-- indexed either -- it is never a lookup key, and the FK's ON DELETE SET NULL
-- scan runs at most once per provider deletion over a table with one row per
-- team.

COMMENT ON TABLE team_ai_summary_settings IS
    'Optional per-team override of the instance AI summary defaults (config.yaml `ai_summary:`). Row absent = inherit instance defaults. The context budgets (per_document_chars, total_context_chars, request_timeout) and max_top_n are deliberately absent: they stay instance-only.';

COMMENT ON COLUMN team_ai_summary_settings.model_provider_id IS
    'Model provider to summarise with. NULL = use the team default provider; set to NULL automatically when the referenced provider is deleted.';

COMMENT ON COLUMN team_ai_summary_settings.top_n IS
    'Documents fed to the summariser, capped at config.MaxAISummaryTopN (10). ai_summary.max_top_n lowers the effective per-team limit inside this ceiling.';
