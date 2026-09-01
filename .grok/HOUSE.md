# Private-dev house (stripped from public Beach)

This checkout's git remote is `ThirdCoastInteractive/the-beach`, branch `master`.
Public squash is `ThirdCoastInteractive/Beach` `main`, identity
`ThirdCoast <git@thirdcoast.tv>`. Private commits:
`Henry Slawniak <henry@slawniak.com>`. No `Co-Authored-By`.

Public product name is **Beach**. Do not put `the-beach`, this remote, `Y:\`,
or `slawniak.com` on a public commit.

Cut: `scripts/release.sh` (parentless first cut / later parent `public/main`).
Guard: `scripts/check-release.sh`. Progress board is the work tracker for
framework cards; chat is not the board.

Grok loads skills from `.grok/skills/`. Canonical authoring is `skills/` at
the repo root (ships). Copy with `python scripts/copy-public-skills.py` if
the two trees drift — that script copies `skills/` → `.grok/skills/`.

`cmd/beach-demos` is private: `go run ./cmd/beach-demos` writes `demos/`.
