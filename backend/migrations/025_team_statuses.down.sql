-- Rollback of 025_team_statuses (issue #1264), in reverse order of the up.
--
-- THIS ROLLBACK IS LOSSY. Once teams have custom statuses, resources carry
-- values the pre-025 schema cannot hold: the re-added CHECKs admit only the
-- system set, and a slug may be longer than varchar(20). So every resource
-- whose status is not one of its type's system statuses is first reassigned to
-- that type's default ('draft' for prompts, 'active' for the other three --
-- models.DefaultStatusFor). Rows already on a system status are not written
-- at all. The custom status definitions themselves go with the table.

-- memories has an unconditional BEFORE UPDATE trigger that rewrites updated_at.
-- Reassigning a status here is not an edit, and updated_at is an edit signal
-- (search recency, resource freshness), so suspend the trigger for the one
-- statement, as 016_resource_labels does. The other three tables have no such
-- trigger.
ALTER TABLE memories DISABLE TRIGGER update_memories_updated_at;

UPDATE memories SET status = 'active'
 WHERE status NOT IN ('active', 'draft', 'archived');

ALTER TABLE memories ENABLE TRIGGER update_memories_updated_at;

UPDATE blueprints SET status = 'active'
 WHERE status NOT IN ('active', 'expired');

UPDATE artifacts SET status = 'active'
 WHERE status NOT IN ('active', 'draft', 'archived');

UPDATE prompts SET status = 'draft'
 WHERE status NOT IN ('draft', 'published');

ALTER TABLE memories ALTER COLUMN status TYPE varchar(20);
ALTER TABLE blueprints ALTER COLUMN status TYPE varchar(20);
ALTER TABLE artifacts ALTER COLUMN status TYPE varchar(20);
ALTER TABLE prompts ALTER COLUMN status TYPE varchar(20);

ALTER TABLE memories
    ADD CONSTRAINT memories_status_check CHECK (status IN ('active', 'draft', 'archived'));
ALTER TABLE prompts
    ADD CONSTRAINT prompts_status_check CHECK (status IN ('draft', 'published'));

DROP TABLE IF EXISTS team_statuses;
