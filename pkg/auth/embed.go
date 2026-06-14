package auth

import "embed"

// Migrations is the embedded goose migration set for the identity tables: users,
// user_credentials_local, the three RBAC tables (roles, role_permissions,
// user_roles), and password_reset_tokens. It ships in the auth skeleton; boot
// wiring passes it to pg.Migrate alongside session.Migrations. The SQL files live
// under a "migrations" directory, as pg.Migrate expects.
//
//go:embed migrations/*.sql
var Migrations embed.FS
