-- Reverses migration 017. Nothing was backfilled, so nothing has to be restored:
-- titles only ever existed in this column and go with it.
ALTER TABLE public.memories DROP COLUMN IF EXISTS title;
