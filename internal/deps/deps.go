// Package deps anchors the module's external dependencies so go.mod stays
// stable while the rest of the framework is built in parallel. Each real
// package below imports what it needs directly; this file just keeps `go mod
// tidy` from pruning a dependency before the package that uses it exists yet.
//
// It can be deleted once every dependency here is imported by a real package.
package deps

import (
	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/a-h/templ"
	_ "github.com/fxamacker/cbor/v2"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/pressly/goose/v3"
	_ "github.com/starfederation/datastar-go/datastar"
	_ "golang.org/x/crypto/argon2"
)
