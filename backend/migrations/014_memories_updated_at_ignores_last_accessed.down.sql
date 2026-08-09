-- Reverses 014 (issue #730): restores the unconditional
-- update_memories_updated_at trigger exactly as 001_baseline created it.
--
-- Rolling back reinstates the behaviour where ANY update to a memories row --
-- including a last-accessed-only write -- moves updated_at. That is the
-- pre-#730 behaviour and is only correct on a schema where nothing writes the
-- last_accessed_* columns.

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    EXECUTE FUNCTION public.update_updated_at_column();
