-- ===========================================================================
-- Instance settings audit -- append-only log of instance-level settings
-- changes (issue #1187, epic #1185).
--
-- Instance email configuration is moving out of config.yaml into the database
-- (#1186) and becomes editable at runtime by any instance admin. Without a log,
-- a credential rotation or a from-address change is unattributable. This table
-- is that record: who changed which instance setting, when, and what it looked
-- like before and after.
--
-- team_settings_audit (015) cannot hold these events: its team_id is NOT NULL
-- and a foreign key, and its readers are tenancy-scoped. It stays team-scoped;
-- this is its instance-level counterpart and mirrors its shape and access
-- pattern minus the team.
--
-- The `setting` column keys the entry, so future instance settings reuse this
-- table rather than adding another. Like 015 it is APPEND-ONLY: the repository
-- exposes no update and no delete.
-- ===========================================================================

CREATE TABLE instance_settings_audit (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    setting       text NOT NULL CHECK (setting <> ''),
    action        text NOT NULL CHECK (action IN ('upsert', 'delete', 'import')),
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    before        jsonb,
    after         jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE instance_settings_audit IS
    'Append-only log of instance-level settings changes (epic #1185). Rows are never updated or deleted.';
COMMENT ON COLUMN instance_settings_audit.setting IS
    'Which instance setting changed, e.g. ''email_provider''. Only non-empty is enforced here, so a new setting needs no migration; the Go constants are the closed set.';
COMMENT ON COLUMN instance_settings_audit.action IS
    'What happened: ''upsert'' (created or replaced), ''delete'', or ''import'' (the boot-time config.yaml import).';
COMMENT ON COLUMN instance_settings_audit.actor_user_id IS
    'Who made the change. NULL for the boot-time import, which has no actor. SET NULL on user deletion rather than CASCADE: the entry must outlive the account, or deleting it would erase the record of what it did.';
COMMENT ON COLUMN instance_settings_audit.before IS
    'Redacted snapshot of the setting before the change. NULL when the change created it. NEVER a credential -- only a "secret": "changed"|"unchanged" marker.';
COMMENT ON COLUMN instance_settings_audit.after IS
    'Redacted snapshot of the setting after the change. NULL when the change deleted it. NEVER a credential -- only a "secret": "changed"|"unchanged" marker.';

-- The audit list is "this setting's entries, newest first", paged by a keyset
-- cursor on (created_at, id).
--
-- `id DESC` is a load-bearing tiebreaker, not cosmetic: `now()` is
-- transaction-start time, so several entries written in one transaction share
-- one created_at, and without the tiebreaker the keyset cursor could not
-- position between them.
CREATE INDEX idx_instance_settings_audit_setting_created
    ON instance_settings_audit (setting, created_at DESC, id DESC);
