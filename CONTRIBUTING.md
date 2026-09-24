# Contributing

Fork [ThirdCoastInteractive/Beach](https://github.com/ThirdCoastInteractive/Beach)
and open a pull request against `main`. Follow [AGENTS.md](AGENTS.md).

- App UI calls `pkg/ui/driftwood`. Do not add HTML primitives the kit already owns.
- SQL lives in sqlc queries. Do not put SQL string literals in Go.
- Datastar attributes come from `pkg/datastar`.
- `make gen` after `.templ` or CSS. `make vet` and `make test` before you stop.
