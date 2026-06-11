-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username   text        NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- Local username/password credentials. password_hash is an argon2id PHC string;
-- it is a secret column and must never appear in a RETURNING clause or SELECT *.
-- +goose StatementBegin
CREATE TABLE user_credentials_local (
    user_id            bigint      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash      text        NOT NULL,
    failed_login_count integer     NOT NULL DEFAULT 0,
    locked_until       timestamptz,
    updated_at         timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- --- RBAC: three tables, flat "resource:action" permissions ---

-- +goose StatementBegin
CREATE TABLE roles (
    id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text   NOT NULL UNIQUE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE role_permissions (
    role_id    bigint NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission text   NOT NULL,                 -- "resource:action"
    PRIMARY KEY (role_id, permission)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE user_roles (
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id bigint NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX user_roles_role_id_idx ON user_roles (role_id);
-- +goose StatementEnd

-- --- password reset: hashed single-use tokens, expiry, audit ---
-- +goose StatementBegin
CREATE TABLE password_reset_tokens (
    token_hash bytea       PRIMARY KEY,          -- SHA256(token); raw token never stored
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE password_reset_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE user_roles;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE role_permissions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE roles;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE user_credentials_local;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd
