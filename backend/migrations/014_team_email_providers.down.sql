-- Reverse 014_team_email_providers (#501). Dropping the table takes its primary
-- key, FK and unique constraint with it.
--
-- This is lossy by nature: every team with its own provider silently reverts to
-- the instance provider from config.yaml, and their configuration -- including
-- the encrypted credential and the delivery health history -- is gone. Mail
-- keeps flowing, but from the operator's address rather than the team's.

DROP TABLE IF EXISTS public.team_email_providers;
