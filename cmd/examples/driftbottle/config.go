// Boot configuration for driftbottle. The live matchmaking/feed are in-memory
// (the hot path), but persistence and analytics are required: Postgres archives
// transcripts and ClickHouse holds the event firehose. Both DSNs are required —
// a missing one aborts boot rather than degrading.
package main

import (
	bcfg "github.com/ThirdCoastInteractive/Beach/pkg/config"
)

// cfg is driftbottle's typed configuration. Core supplies Port (default 8080),
// DSN (POSTGRES_DSN|DATABASE_URL — required, no default), AppEnv, and BaseURL;
// CHDSN is the ClickHouse firehose, also required. So both Postgres (Core.DSN)
// and ClickHouse (CHDSN) must be present for the process to start.
type cfg struct {
	bcfg.Core
	CHDSN string `env:"CLICKHOUSE_DSN,required"`
}

// loadConfig loads and validates cfg from the environment, aborting with the
// full list of missing required variables if anything is unset.
func loadConfig() cfg { return *bcfg.MustLoad[cfg]() }
