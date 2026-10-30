-- Model persistence and operation history (NF-8, HI-1, ADR-0003).
-- Workspace metadata and user foreign keys arrive with M3/M4.
-- +goose Up
CREATE TABLE workspaces (
    id uuid PRIMARY KEY,
    seq bigint NOT NULL DEFAULT 0 CHECK (seq >= 0)
);

CREATE TABLE elements (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('person', 'softwareSystem', 'container', 'component')),
    parent_id uuid,
    name text NOT NULL CHECK (btrim(name) <> ''),
    dsl_id text NOT NULL CHECK (btrim(dsl_id) <> ''),
    attrs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(attrs) = 'object'),
    PRIMARY KEY (workspace_id, id),
    -- Batches may swap DSL identifiers or create a child before its parent.
    UNIQUE (workspace_id, dsl_id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id, parent_id) REFERENCES elements(workspace_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    CHECK ((type IN ('person', 'softwareSystem') AND parent_id IS NULL)
        OR (type IN ('container', 'component') AND parent_id IS NOT NULL))
);
CREATE INDEX elements_parent_idx ON elements (workspace_id, parent_id);

CREATE TABLE relationships (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    source_id uuid NOT NULL,
    destination_id uuid NOT NULL,
    attrs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(attrs) = 'object'),
    PRIMARY KEY (workspace_id, id),
    FOREIGN KEY (workspace_id, source_id) REFERENCES elements(workspace_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id, destination_id) REFERENCES elements(workspace_id, id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX relationships_source_idx ON relationships (workspace_id, source_id);
CREATE INDEX relationships_destination_idx ON relationships (workspace_id, destination_id);

CREATE TABLE operations (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    seq bigint NOT NULL CHECK (seq > 0),
    batch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    ops jsonb NOT NULL CHECK (jsonb_typeof(ops) = 'array'),
    PRIMARY KEY (workspace_id, seq),
    UNIQUE (workspace_id, batch_id)
);
