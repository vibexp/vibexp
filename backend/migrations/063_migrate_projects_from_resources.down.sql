-- Down migration for 063: Remove migrated projects
-- This migration removes all projects that were created by the migration
-- We identify these by checking if they have the default empty values for
-- description, git_url, and homepage (which indicates they were auto-migrated)

-- Note: This down migration is safe because:
-- 1. We haven't established foreign key relationships yet
-- 2. The original project_name fields in artifacts and spec_library remain unchanged
-- 3. Users haven't had a chance to customize these projects yet (description, git_url, homepage are empty)

DELETE FROM projects
WHERE description = ''
  AND git_url = ''
  AND homepage = ''
  AND version = 1;
