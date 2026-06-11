-- Pantry domain schema: storage locations, inventory items, shopping lists.
--
-- The identity/session/RBAC tables (users, user_credentials_local, roles,
-- role_permissions, user_roles, sessions, password_reset_tokens) are supplied by
-- the framework's own embedded migration sets (auth.Migrations + session.Migrations),
-- which boot wiring runs ahead of this file. This migration owns only the
-- household-pantry tables and seeds the two roles the app uses.

-- +goose Up

-- Storage locations: pantry shelf, fridge, freezer, garage, ...
-- +goose StatementBegin
CREATE TABLE pantry_locations (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text        NOT NULL,
    kind       text        NOT NULL DEFAULT 'pantry', -- pantry|fridge|freezer|other
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
-- +goose StatementEnd

-- Inventory items. quantity + unit, optional expiry, optional photo URL. The
-- photo is rendered through the ui image / aspect-ratio components (no layout
-- shift); we store only its URL here.
-- +goose StatementBegin
CREATE TABLE pantry_items (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text        NOT NULL,
    quantity    numeric     NOT NULL DEFAULT 1,
    unit        text        NOT NULL DEFAULT 'ea',
    location_id bigint      REFERENCES pantry_locations (id) ON DELETE SET NULL,
    category    text        NOT NULL DEFAULT 'other',
    photo_url   text        NOT NULL DEFAULT '',
    expires_at  date,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX pantry_items_location_idx ON pantry_items (location_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX pantry_items_expires_idx ON pantry_items (expires_at);
-- +goose StatementEnd

-- Shopping lists and their lines.
-- +goose StatementBegin
CREATE TABLE pantry_lists (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE pantry_list_items (
    id       bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    list_id  bigint  NOT NULL REFERENCES pantry_lists (id) ON DELETE CASCADE,
    name     text    NOT NULL,
    quantity numeric NOT NULL DEFAULT 1,
    checked  boolean NOT NULL DEFAULT false
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX pantry_list_items_list_idx ON pantry_list_items (list_id);
-- +goose StatementEnd

-- Seed the two roles the app gates on. Permissions are flat "resource:action"
-- strings the auth layer flattens onto the Principal at login.
-- household-admin can write inventory and manage members; household-member can
-- only read.
-- +goose StatementBegin
INSERT INTO roles (slug) VALUES ('household-admin'), ('household-member');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO role_permissions (role_id, permission)
SELECT id, perm
  FROM roles
  CROSS JOIN (VALUES
    ('pantry:read'),
    ('pantry:write'),
    ('pantry:admin')
  ) AS p(perm)
 WHERE roles.slug = 'household-admin';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO role_permissions (role_id, permission)
SELECT id, 'pantry:read'
  FROM roles
 WHERE roles.slug = 'household-member';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS pantry_list_items;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS pantry_lists;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS pantry_items;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS pantry_locations;
-- +goose StatementEnd
