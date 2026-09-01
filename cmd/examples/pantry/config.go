package main

import bcfg "github.com/ThirdCoastInteractive/Beach/pkg/config"

// appConfig is pantry's typed env, loaded once at boot by the framework config
// loader. It embeds Core (AppEnv, Port, DSN via POSTGRES_DSN|DATABASE_URL — which
// is required, no default) and requires a ClickHouse DSN. pantry needs BOTH
// stores: Postgres is the system of record (inventory, auth), ClickHouse is the
// activity firehose behind the dashboard. There is no database-less mode.
type appConfig struct {
	bcfg.Core
	ClickhouseDSN string `env:"CLICKHOUSE_DSN,required"`
}

// loadConfig loads and validates the environment, aborting with the missing list
// if Postgres or ClickHouse is unconfigured.
func loadConfig() appConfig { return *bcfg.MustLoad[appConfig]() }
