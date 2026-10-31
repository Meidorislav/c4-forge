-- Presentation entities for diagrams and freeform annotations (ADR-0003).
-- +goose Up
CREATE TABLE views (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('systemLandscape','systemContext','container','component')),
    key text NOT NULL CHECK (btrim(key) <> ''),
    scope_id uuid,
    attrs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(attrs) = 'object'),
    PRIMARY KEY (workspace_id,id),
    UNIQUE (workspace_id,key) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id,scope_id) REFERENCES elements(workspace_id,id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    CHECK ((type = 'systemLandscape' AND scope_id IS NULL) OR (type <> 'systemLandscape' AND scope_id IS NOT NULL))
);
CREATE INDEX views_scope_idx ON views(workspace_id,scope_id);

CREATE TABLE placements (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    view_id uuid NOT NULL,
    element_id uuid NOT NULL,
    x double precision NOT NULL CHECK (x NOT IN ('Infinity'::float8, '-Infinity'::float8, 'NaN'::float8)),
    y double precision NOT NULL CHECK (y NOT IN ('Infinity'::float8, '-Infinity'::float8, 'NaN'::float8)),
    width double precision CHECK (width > 0 AND width NOT IN ('Infinity'::float8, 'NaN'::float8)),
    height double precision CHECK (height > 0 AND height NOT IN ('Infinity'::float8, 'NaN'::float8)),
    PRIMARY KEY (workspace_id,id),
    UNIQUE (workspace_id,view_id,element_id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id,view_id) REFERENCES views(workspace_id,id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id,element_id) REFERENCES elements(workspace_id,id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX placements_element_idx ON placements(workspace_id,element_id);

CREATE TABLE shapes (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    view_id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('sticky','rectangle','ellipse','text','arrow')),
    attrs jsonb NOT NULL CHECK (jsonb_typeof(attrs) = 'object' AND attrs ? 'geometry' AND jsonb_typeof(attrs->'geometry') = 'object'),
    PRIMARY KEY (workspace_id,id),
    FOREIGN KEY (workspace_id,view_id) REFERENCES views(workspace_id,id)
        ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX shapes_view_idx ON shapes(workspace_id,view_id);
