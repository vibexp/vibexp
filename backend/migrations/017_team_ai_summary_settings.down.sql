-- Reverse 017_team_ai_summary_settings (#1071). Dropping the table takes its
-- CHECK constraints, its foreign keys and its comments with it; nothing else
-- referenced it.

DROP TABLE IF EXISTS team_ai_summary_settings;
