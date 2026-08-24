-- ===========================================================================
-- Team settings audit -- append-only log of settings-copy events (issue #828,
-- epic #827).
--
-- A cross-team settings copy moves a credential's *use* into a different set
-- of members, and the person performing it need not be one of the destination
-- team's owners. This log is the compensating control: it is what lets a team's
-- owners see, after the fact, that a provider configuration arrived from
-- somewhere else and who brought it.
--
-- `internal/services/activities/` cannot serve this and was evaluated and
-- rejected (epic #827, decision 8). It has no team_id column at all
-- (001_baseline.up.sql:84-96), so there is no query that returns "this team's
-- events"; its reads are hard-scoped to the ACTING USER, so a team owner cannot
-- see an admin's copy; and its rows expire on `retention.activity_days`, which
-- makes it a recent-activity feed rather than an audit trail. The right
-- precedent is `resource_freshness_audit` (013_consolidated.up.sql), and this
-- table deliberately mirrors its shape and access pattern.
--
-- Like that table it is APPEND-ONLY: the repository exposes no update and no
-- delete, and rows disappear only with their team.
-- ===========================================================================

CREATE TABLE team_settings_audit (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id             uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    actor_user_id       uuid REFERENCES users (id) ON DELETE SET NULL,
    surface             text NOT NULL,
    source_team_id      uuid,
    source_resource_id  uuid,
    created_resource_id uuid,
    detail              jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE team_settings_audit IS
    'Append-only log of team settings-copy events (epic #827). Rows are never updated or deleted except by team cascade.';
COMMENT ON COLUMN team_settings_audit.team_id IS
    'The DESTINATION team -- the team that now owns the copied configuration, and whose owners the log is written for. Cascades, because an entry about a team that no longer exists is unreadable.';
COMMENT ON COLUMN team_settings_audit.actor_user_id IS
    'Who performed the copy. SET NULL on user deletion rather than CASCADE: the entry must outlive the actor, since deleting the account would otherwise erase the record of what that account did.';
COMMENT ON COLUMN team_settings_audit.surface IS
    'Which settings surface was copied: ''embedding_provider'', ''model_provider'' or ''custom_types''. Validated in the service layer, mirroring resource_freshness_audit.action.';
COMMENT ON COLUMN team_settings_audit.source_team_id IS
    'The team the configuration was copied FROM. Deliberately NO foreign key: the log must survive the source team being deleted, which a cascade would erase and a SET NULL would silently blank. NULL only when the source is genuinely unknown.';
COMMENT ON COLUMN team_settings_audit.source_resource_id IS
    'The row that was copied, in the source team. No foreign key: it is polymorphic across the provider and type tables (the constraint resource_freshness and resource_relations live with), and the entry must survive that row being deleted or rotated away.';
COMMENT ON COLUMN team_settings_audit.created_resource_id IS
    'The row the copy produced, in the destination team. NULL when one action created several rows (a custom-types copy) -- the individual ids are then in detail.';
COMMENT ON COLUMN team_settings_audit.detail IS
    'Per-surface context: names, the provider type, how many types were copied and which slugs were skipped. NEVER a credential -- ciphertext moves server-side and no part of it is logged. Defaults to an empty object so readers never have to distinguish NULL from "nothing recorded".';

-- The audit list is "this team's entries, newest first" -- the same access
-- pattern as idx_resource_freshness_audit_team_created, and the read endpoint
-- (#832) pages on it.
--
-- `id DESC` is a deterministic tiebreaker and it is load-bearing rather than
-- cosmetic: `now()` is transaction-start time, so one copy action that writes
-- several entries in a single transaction stamps them all identically. Without
-- the tiebreaker their relative order is undefined between queries and
-- pagination can repeat or skip entries.
CREATE INDEX idx_team_settings_audit_team_created
    ON team_settings_audit (team_id, created_at DESC, id DESC);
