-- 018_prompt_references_team_scope (#1100): data-only correction of the stored
-- prompt reference graph.
--
-- Until #1100 a prompt's @slug references were resolved among its AUTHOR's own
-- prompts in ANY team, never within the team that owns the prompt. That left
-- prompt_references with two kinds of wrong row, both of which this fixes:
--   1. edges into another team (content of team Y wired into a team X prompt);
--   2. missing edges to a teammate's prompt, so deleting that prompt was not
--      blocked by HasDependents.
-- Self-edges are dropped too: the service no longer stores a prompt as its own
-- dependency.
--
-- The INSERT mirrors the Go extraction in PromptService.updatePromptReferences:
-- '@@' is an escaped literal '@' and never a reference, then every
-- @([a-zA-Z0-9_-]+) resolves to the prompt with that slug in the SAME team
-- (slugs are unique per team: prompts_slug_team_id_key). '@@' is replaced with a
-- space rather than removed so the text on either side cannot join into a
-- different slug ('@a@@b' must yield 'a', not 'ab').
--
-- Idempotent: the DELETE finds nothing on a second run and the INSERT is
-- ON CONFLICT DO NOTHING against prompt_references' (prompt_id,
-- referenced_prompt_id) unique constraint.

DELETE FROM prompt_references pr
USING prompts a, prompts b
WHERE pr.prompt_id = a.id
  AND pr.referenced_prompt_id = b.id
  AND (a.team_id IS DISTINCT FROM b.team_id OR a.id = b.id);

INSERT INTO prompt_references (prompt_id, referenced_prompt_id)
SELECT DISTINCT p.id, r.id
FROM prompts p
CROSS JOIN LATERAL regexp_matches(replace(p.body, '@@', ' '), '@([a-zA-Z0-9_-]+)', 'g') AS m(slug)
JOIN prompts r
  ON r.team_id = p.team_id
 AND r.slug = m.slug[1]
 AND r.id <> p.id
ON CONFLICT (prompt_id, referenced_prompt_id) DO NOTHING;
