-- Revert typed resource relations (issue #422, epic #421).
-- Dropping the table removes its indexes with it.
DROP TABLE IF EXISTS resource_relations;
