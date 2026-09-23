-- Reverse 018_prompt_references_team_scope (#1100): intentionally a no-op.
--
-- 018 is a data-only correction of prompt_references (dropping cross-team and
-- self edges, adding missed same-team edges). The rows it removed were wrong and
-- cannot be told apart from correct ones afterwards, so there is nothing
-- meaningful to restore; the schema is unchanged either way.

SELECT 1;
