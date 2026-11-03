-- Workspace metadata and explicit membership (WS-1, WS-2, ADR-0006).
-- +goose Up
ALTER TABLE workspaces
    ADD COLUMN name text CHECK (name IS NULL OR char_length(name) BETWEEN 1 AND 200),
    ADD COLUMN description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 4000),
    ADD COLUMN created_by uuid REFERENCES users(id),
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id),
    role text NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user_idx ON workspace_members(user_id, workspace_id);

-- Keep metadata and model writes on the same last-modified clock.
-- +goose StatementBegin
CREATE FUNCTION touch_workspace() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.seq IS DISTINCT FROM OLD.seq OR NEW.name IS DISTINCT FROM OLD.name
       OR NEW.description IS DISTINCT FROM OLD.description THEN
        NEW.updated_at := clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER workspace_modified BEFORE UPDATE ON workspaces
FOR EACH ROW EXECUTE FUNCTION touch_workspace();
