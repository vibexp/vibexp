-- Reverse 012_team_search_settings (#488). Dropping the table takes its CHECK
-- constraints and primary-key index with it.
--
-- This is lossy by nature: every overriding team silently reverts to the
-- instance defaults from config.yaml, and their tuned profiles are gone.

DROP TABLE IF EXISTS team_search_settings;
