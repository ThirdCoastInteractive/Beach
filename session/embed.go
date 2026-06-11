package session

import "embed"

// Migrations is the embedded goose migration set for the sessions table. It
// ships in the auth skeleton alongside the users/credentials/RBAC tables; the
// app's boot wiring passes it to pg.Migrate (the SQL files live under a
// "migrations" directory, as pg.Migrate expects).
//
//go:embed migrations/*.sql
var Migrations embed.FS
