-- ===========================================================================
-- Typed resource relations: directed, typed edges between the four resource
-- types (artifact, memory, prompt, blueprint) within a project (issue #422,
-- epic #421 "Typed resource relations").
--
-- Polymorphic table following the comments precedent (migration 006): the
-- (from_type, from_id) and (to_type, to_id) pairs identify the two endpoints.
-- Neither *_id carries a foreign key -- each spans four resource tables, so
-- endpoint cleanup is app-level (each resource service's delete path removes
-- every edge the resource appears on; cf. comments' DeleteByResource). team_id
-- and project_id DO carry FKs (ON DELETE CASCADE) so an edge dies with its team
-- or project; created_by / confirmed_by carry FKs (ON DELETE SET NULL) so an
-- edge outlives the user who authored or confirmed it.
--
-- An edge carries intent that similarity cannot: relation_type names the
-- obligation/lineage ('governed-by', 'built-from', 'explained-by',
-- 'supersedes'); origin records whether a human or the AI proposed it; status
-- records the tiered-trust lifecycle ('suggested' -> 'confirmed'). The object
-- (to) type is constrained per relation_type in the service layer, not here.
-- ===========================================================================

CREATE TABLE resource_relations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id    uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    from_type     text NOT NULL CHECK (from_type IN ('artifact', 'memory', 'prompt', 'blueprint')),
    from_id       uuid NOT NULL,
    to_type       text NOT NULL CHECK (to_type IN ('artifact', 'memory', 'prompt', 'blueprint')),
    to_id         uuid NOT NULL,
    relation_type text NOT NULL CHECK (relation_type IN ('governed-by', 'supersedes', 'built-from', 'explained-by')),
    origin        text NOT NULL CHECK (origin IN ('ai', 'human')),
    status        text NOT NULL CHECK (status IN ('suggested', 'confirmed')),
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    confirmed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- One edge per (subject, relation_type, object) within a team: makes duplicate
-- creation a no-op that returns the existing row (idempotent Create).
CREATE UNIQUE INDEX idx_resource_relations_unique
    ON resource_relations (team_id, from_type, from_id, relation_type, to_type, to_id);

-- Outgoing lookups: "what does this resource point at" (the from side).
CREATE INDEX idx_resource_relations_from
    ON resource_relations (team_id, from_type, from_id);

-- Incoming lookups: "what points at this resource" (the to side).
CREATE INDEX idx_resource_relations_to
    ON resource_relations (team_id, to_type, to_id);
