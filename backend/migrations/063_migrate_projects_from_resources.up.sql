-- Migration 063: Migrate existing project names to centralized projects table
-- This migration extracts project names from artifacts and spec_library tables
-- and creates corresponding entries in the projects table

-- Step 1: Create a temporary function to generate unique slugs
CREATE OR REPLACE FUNCTION generate_unique_project_slug(p_user_id UUID, p_name VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    base_slug VARCHAR;
    final_slug VARCHAR;
    counter INT := 1;
BEGIN
    -- Generate base slug: lowercase, replace spaces/special chars with hyphens
    base_slug := lower(regexp_replace(p_name, '[^a-zA-Z0-9]+', '-', 'g'));
    -- Trim leading/trailing hyphens and collapse multiple hyphens
    base_slug := regexp_replace(base_slug, '^-+|-+$', '', 'g');
    base_slug := regexp_replace(base_slug, '-+', '-', 'g');

    -- Handle empty slug (special characters only)
    IF base_slug = '' OR base_slug IS NULL THEN
        base_slug := 'project';
    END IF;

    -- Ensure slug doesn't exceed 100 chars (projects.slug is VARCHAR(100))
    IF length(base_slug) > 90 THEN
        base_slug := substring(base_slug, 1, 90);
    END IF;

    final_slug := base_slug;

    -- Check for collisions and append counter if needed
    WHILE EXISTS (SELECT 1 FROM projects WHERE user_id = p_user_id AND slug = final_slug) LOOP
        counter := counter + 1;
        final_slug := base_slug || '-' || counter;
    END LOOP;

    RETURN final_slug;
END;
$$ LANGUAGE plpgsql;

-- Step 2: Create a temporary table to collect all unique (user_id, project_name) combinations
CREATE TEMP TABLE temp_projects_to_migrate (
    user_id UUID,
    project_name VARCHAR(255),
    earliest_created_at TIMESTAMP WITH TIME ZONE
);

-- Step 3: Collect unique projects from artifacts
INSERT INTO temp_projects_to_migrate (user_id, project_name, earliest_created_at)
SELECT
    a.user_id,
    CASE
        WHEN a.project_name = '' OR a.project_name IS NULL THEN 'shared'
        ELSE a.project_name
    END as project_name,
    MIN(a.created_at) as earliest_created_at
FROM artifacts a
GROUP BY a.user_id,
    CASE
        WHEN a.project_name = '' OR a.project_name IS NULL THEN 'shared'
        ELSE a.project_name
    END;

-- Step 4: Collect unique projects from spec_library (avoiding duplicates)
INSERT INTO temp_projects_to_migrate (user_id, project_name, earliest_created_at)
SELECT
    s.user_id,
    CASE
        WHEN s.project_name = '' OR s.project_name IS NULL THEN 'shared'
        ELSE s.project_name
    END as project_name,
    MIN(s.created_at) as earliest_created_at
FROM spec_library s
WHERE NOT EXISTS (
    SELECT 1
    FROM temp_projects_to_migrate t
    WHERE t.user_id = s.user_id
      AND t.project_name = CASE
        WHEN s.project_name = '' OR s.project_name IS NULL THEN 'shared'
        ELSE s.project_name
      END
)
GROUP BY s.user_id,
    CASE
        WHEN s.project_name = '' OR s.project_name IS NULL THEN 'shared'
        ELSE s.project_name
    END;

-- Step 5: Collect unique projects from memories metadata (if they contain project references)
INSERT INTO temp_projects_to_migrate (user_id, project_name, earliest_created_at)
SELECT
    m.user_id,
    m.metadata->>'project_name' as project_name,
    MIN(m.created_at) as earliest_created_at
FROM memories m
WHERE m.metadata->>'project_name' IS NOT NULL
  AND m.metadata->>'project_name' != ''
  AND NOT EXISTS (
    SELECT 1
    FROM temp_projects_to_migrate t
    WHERE t.user_id = m.user_id
      AND t.project_name = m.metadata->>'project_name'
  )
GROUP BY m.user_id, m.metadata->>'project_name';

-- Step 6: Insert projects from the temporary table into the projects table
INSERT INTO projects (id, user_id, name, slug, description, git_url, homepage, created_at, updated_at, version)
SELECT
    uuid_generate_v4() as id,
    t.user_id,
    t.project_name as name,
    generate_unique_project_slug(t.user_id, t.project_name) as slug,
    '' as description,
    '' as git_url,
    '' as homepage,
    COALESCE(t.earliest_created_at, CURRENT_TIMESTAMP) as created_at,
    CURRENT_TIMESTAMP as updated_at,
    1 as version
FROM temp_projects_to_migrate t
WHERE NOT EXISTS (
    -- Double-check to avoid any race conditions or conflicts
    SELECT 1 FROM projects p
    WHERE p.user_id = t.user_id
      AND (p.name = t.project_name OR p.slug = generate_unique_project_slug(t.user_id, t.project_name))
)
ORDER BY t.user_id, t.earliest_created_at;

-- Step 7: Drop the temporary function and table
DROP FUNCTION IF EXISTS generate_unique_project_slug(UUID, VARCHAR);
DROP TABLE IF EXISTS temp_projects_to_migrate;
