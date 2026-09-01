# Contributing

Private development is on `origin/master` (`ThirdCoastInteractive/the-beach`).
Public cuts are squashed onto `public/main` (`ThirdCoastInteractive/Beach`) by
`scripts/release.sh`. Identity on public commits is `ThirdCoast <git@thirdcoast.tv>`.

`scripts/check-release.sh` is the source of truth for what a public commit may
contain. Do not invent extra rules.

- App UI calls `pkg/ui/driftwood`. Do not add HTML primitives the kit already owns.
- SQL lives in sqlc queries. Do not put SQL string literals in Go.
- Datastar attributes come from `pkg/datastar`.
- `make gen` after `.templ` or CSS. `make vet` and `make test` before you stop.
