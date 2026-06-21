-- Step 1: Add project_id column (nullable initially for migration)
ALTER TABLE memories ADD COLUMN project_id UUID;

-- Step 2: Create a default project per (user_id, team_id) pair seen in memories,
-- where no project exists for that pair yet.
-- Slug is suffixed with team_id prefix to avoid slug collisions within the same user
-- (slug uniqueness is per (user_id, slug) per migration 062).
INSERT INTO projects (id, user_id, team_id, name, slug, description, created_at, updated_at)
SELECT
    uuid_generate_v4(),
    m.user_id,
    m.team_id,
    'Default Project',
    'default-project-' || substr(m.team_id::text, 1, 8),
    'Default project for migrated memories',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM (
    SELECT DISTINCT user_id, team_id FROM memories
) m
WHERE NOT EXISTS (
    SELECT 1 FROM projects p
    WHERE p.user_id = m.user_id AND p.team_id = m.team_id
);

-- Step 3: Backfill — assign each memory to the oldest project in its (user_id, team_id) scope
UPDATE memories m
SET project_id = (
    SELECT p.id
    FROM projects p
    WHERE p.user_id = m.user_id AND p.team_id = m.team_id
    ORDER BY p.created_at ASC
    LIMIT 1
)
WHERE m.project_id IS NULL;

-- Step 4: Abort if any rows remain unassigned (do NOT silently delete)
DO $$
DECLARE
    orphan_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO orphan_count FROM memories WHERE project_id IS NULL;
    IF orphan_count > 0 THEN
        RAISE EXCEPTION 'Migration 096 failed: % memories could not be assigned to a project. Aborting.', orphan_count;
    END IF;
END $$;

-- Step 5: Make project_id NOT NULL after data migration
ALTER TABLE memories ALTER COLUMN project_id SET NOT NULL;

-- Step 6: Add foreign key constraint with cascade delete
ALTER TABLE memories ADD CONSTRAINT fk_memories_project_id
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Step 7: Add index for performance
CREATE INDEX idx_memories_project_id ON memories(project_id);
