package main

import bcfg "github.com/ThirdCoastInteractive/Beach/pkg/config"

// cfg is boardwalk's typed env, loaded once at boot by the framework config
// loader. It embeds Core (AppEnv, Port, DSN via POSTGRES_DSN|DATABASE_URL —
// which is required, no default) and requires a ClickHouse DSN. boardwalk needs
// BOTH stores: Postgres holds the periodic CBOR snapshot (restored on boot) and
// ClickHouse holds the game-action firehose behind /stats. The live game still
// runs entirely in the in-memory ecs.Store/sim — but a missing DSN aborts boot
// rather than degrading, so the persistence and analytics lanes are never
// silently off.
type cfg struct {
	bcfg.Core
	CHDSN string `env:"CLICKHOUSE_DSN,required"`
}

// loadConfig loads and validates the environment, aborting with the missing
// list if Postgres or ClickHouse is unconfigured.
func loadConfig() cfg { return *bcfg.MustLoad[cfg]() }
