-- Blueprint sync foundation (issue #339, epic #334: Sync-Ready Blueprints).
--
-- Gives every blueprint a canonical repo-relative path, its original raw bytes,
-- a content hash, and import provenance — the schema gate the rest of the epic
-- stores into and reads from. Also adds raw_content to content_versions so
-- version snapshot/restore round-trips raw (#340).
--
-- NOTE ON NUMBERING: the epic issue text calls this "migration 008", but the
-- post-v0.6.0 migrations were consolidated into 006 (#399), so the next free
-- slot is 007. Renumbered accordingly; behavior is unchanged.
--
-- Strategy: add all columns nullable, backfill deterministically, then tighten
-- `path` to NOT NULL + UNIQUE(project_id, path). Legacy paths are DERIVED from
-- (type, subtype, slug) via a CASE that mirrors the Go blueprintpath.DefaultPath
-- reverse mapping (#335); a Go parity test pins the two together.

-- pgcrypto provides digest() for the content_sha backfill (idempotent).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1. Add the new columns, all nullable so the backfill can run before the
--    NOT NULL/UNIQUE tightening. path_derived defaults true: every backfilled
--    row is a derived path; the app sets it explicitly for new rows.
ALTER TABLE public.blueprints
    ADD COLUMN path text,
    ADD COLUMN path_derived boolean NOT NULL DEFAULT true,
    ADD COLUMN raw_content text,
    ADD COLUMN content_sha character varying(64),
    ADD COLUMN source_repo text,
    ADD COLUMN source_commit_sha character varying(64),
    ADD COLUMN source_blob_sha character varying(64),
    ADD COLUMN imported_at timestamp with time zone;

ALTER TABLE public.content_versions
    ADD COLUMN raw_content text;

-- 2. Backfill raw_content (= current parsed content; no frontmatter
--    reconstruction for legacy rows) and content_sha = sha256(raw_content).
--    Guarded by WHERE ... IS NULL so it is safe/idempotent.
UPDATE public.blueprints
SET raw_content = content,
    content_sha = encode(digest(content, 'sha256'), 'hex')
WHERE raw_content IS NULL;

-- 3. Backfill path: derive a base path from (type, subtype, slug) via the shared
--    reverse mapping, then suffix conflicts within (project_id, base_path) with
--    -2, -3, ... inserted before the file extension. Suffixing changes only the
--    path, never the slug, so both slug-uniqueness scopes are untouched.
WITH derived AS (
    SELECT
        id,
        project_id,
        CASE
            WHEN type = 'claude' AND COALESCE(subtype, '') = 'claude-md' THEN 'CLAUDE.md'
            WHEN type = 'cursor' AND COALESCE(subtype, '') = 'cursor-md' THEN 'CURSOR.md'
            WHEN type = 'codex' AND COALESCE(subtype, '') = 'agents-md' THEN 'AGENTS.md'
            WHEN type = 'claude-code' AND COALESCE(subtype, '') = 'sub-agents' THEN '.claude/agents/' || slug || '.md'
            WHEN type = 'claude-code' AND COALESCE(subtype, '') = 'skills' THEN '.claude/skills/' || slug || '/SKILL.md'
            WHEN type = 'claude-code' AND COALESCE(subtype, '') = 'slash-commands'
                THEN '.claude/commands/' || slug || '.md'
            WHEN type = 'claude-code' AND COALESCE(subtype, '') = 'others' THEN '.claude/' || slug || '.md'
            WHEN type = 'cursor' AND COALESCE(subtype, '') = 'skills' THEN '.cursor/skills/' || slug || '/SKILL.md'
            WHEN type = 'cursor' AND COALESCE(subtype, '') = 'agents' THEN '.cursor/agents/' || slug || '.md'
            WHEN type = 'cursor' AND COALESCE(subtype, '') = 'commands' THEN '.cursor/commands/' || slug || '.md'
            WHEN type = 'cursor' AND COALESCE(subtype, '') = 'rules' THEN '.cursor/rules/' || slug || '.mdc'
            WHEN type = 'codex' AND COALESCE(subtype, '') = 'rules' THEN '.codex/rules/' || slug || '.md'
            WHEN type = 'codex' AND COALESCE(subtype, '') = 'skills' THEN '.codex/skills/' || slug || '/SKILL.md'
            WHEN type = 'codex' AND COALESCE(subtype, '') = 'others' THEN '.codex/' || slug || '.md'
            ELSE slug || '.md'
        END AS base_path
    FROM public.blueprints
    WHERE path IS NULL
),
ranked AS (
    SELECT
        id,
        base_path,
        row_number() OVER (PARTITION BY project_id, base_path ORDER BY id) AS rn
    FROM derived
)
UPDATE public.blueprints b
SET path = CASE
        WHEN r.rn = 1 THEN r.base_path
        -- Insert "-<rn>" before the trailing file extension (or at the end when
        -- there is none): foo.md -> foo-2.md, SKILL.md -> SKILL-2.md.
        ELSE regexp_replace(r.base_path, '(\.[^./]+)?$', '-' || r.rn || '\1')
    END
FROM ranked r
WHERE b.id = r.id;

-- Log any conflicts that had to be suffixed so operators can see them.
DO $$
DECLARE
    conflict_count integer;
BEGIN
    SELECT count(*) INTO conflict_count
    FROM (
        SELECT project_id, path, count(*) AS c
        FROM public.blueprints
        GROUP BY project_id, path
        HAVING count(*) > 1
    ) dupes;
    IF conflict_count > 0 THEN
        RAISE NOTICE 'blueprint path backfill: % (project_id, path) groups still collide after suffixing', conflict_count;
    END IF;
END $$;

-- 4. Tighten: path is now populated for every row.
ALTER TABLE public.blueprints ALTER COLUMN path SET NOT NULL;
ALTER TABLE public.blueprints
    ADD CONSTRAINT blueprints_project_id_path_unique UNIQUE (project_id, path);
