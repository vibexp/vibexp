-- Revert resource comments (issue #273).
--
-- Dropping the table removes its two indexes with it. resource_id had no FK, so
-- there is nothing else to unwind; team_id/user_id FKs are dropped with the table.
DROP TABLE IF EXISTS comments;
