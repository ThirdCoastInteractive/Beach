---
name: beach-test
description: >
  Write and place tests the Beach way: integration over unit, real Postgres and
  a real ecs.Store over mocks, a regression before every bug fix, and laws
  expressed as tests rather than review notes. Use when adding or changing any
  _test.go file, when a component/chart/handler is added, when a bug is being
  fixed, or when the user mentions tests, coverage, regression, golden files,
  mocks, fixtures, playwright, perf budget, or runs /beach-test. Do not add a
  mocking framework, an assertion library, or a golden-file snapshot of markup.
---

# Beach tests

Follow the `beach` skill first ([../beach/SKILL.md](../beach/SKILL.md)). Doctrine: [10-tooling.md](../../docs/architecture/10-tooling.md).

Tests here are colocated, table-driven, and dependency-light. There is no `test/` tree, no testify, no gomock, no golden markup files. `t.Run` subtests and a `map[string]tc` table are the whole vocabulary.

## Doctrine

- **Integration over unit.** The grug sweet spot: sim tests use a real `ecs.Store`, db tests a real Postgres. Mock only what you cannot run.
- **Regression before fix.** Reproduce the bug as a failing test, then fix it. A fix with no test is a bug waiting to come back.
- **Determinism is the feature.** Sim tests read `construct → Send → Tick(n) → assert`. No sleeps, no wall clock.
- **A law is a test.** When a rule applies to *every* component or *every* token, do not write it in a doc and hope. Write the check and run it over the existing case maps — see below.

## Where a test goes

| Change | Test |
| --- | --- |
| driftwood component | `renderCase` map in `driftwood_test.go` (or `catalog_test.go`) **and** the zero-value map in `a11y_test.go` |
| chart | `chart/a11y_test.go` case map; geometry gets its own `Layout*` test |
| handler shape / guard / patch | `pkg/beach/*_test.go` with `httptest` + the `testApp(t)` helper |
| lint rule | a planted violation in `internal/lint/testdata/bad/`, plus a count in `lint_test.go` |
| CSS law | a test that reads `view.InputCSS()` / `view.Tokens()` — the stylesheet is readable from Go for exactly this |
| a11y | see the `beach-a11y` skill |

## The case-map pattern

Most kit suites are one map walked by one loop, so a new component is covered the moment it is added to the map — and *not* covered if it is not.

```go
cases := map[string]renderCase{
    "card": {c: Card(CardProps{Heading: "H"}), children: []templ.Component{text("body")}},
}
for name, tc := range cases {
    out := render(t, tc.c, tc.children...)
    noColorLiteral(t, name, out)
    referencesResolve(t, name, out)
    // …one call per law
}
```

Add the component to the map. Do not write a bespoke `TestMyComponent` that checks the one thing you were thinking about.

**Render it twice**: once as it is meant to be used, and once from `ComponentProps{}`. The second is the suite that earns its keep — a component that names itself from an optional prop is conformant when the prop is set and silently broken when it is not.

## Prove the test bites

A test that cannot fail is worse than no test, because it reads as coverage. After writing a law, break the thing it guards, watch it fail, then revert. The reduced-motion and live-region laws in this repo were both confirmed that way, and one of them found a real gap on its first run.

## Not tests

| Concern | How it is actually checked |
| --- | --- |
| 14KB first response | runtime dev warning + the examples' perf JSONL gate (`internal/perf`), tracked in git so trends show in diffs |
| console errors, 404s, visible change | a real browser — `make up` then click the flow. A rendered page is not a working feature |
| screen-reader output | not automated; the accessibility tree is the closest a test gets |
| SSE fan-out under load | `driftbottle`'s 5k-subscriber benchmark |

Playwright e2e runs `workers=1`: SSE handshakes do not tolerate parallel handshakes against one container.

## Commands

```
make test              # depends on gen + build
go test ./pkg/ui/...   # the kit's laws
go run ./cmd/beach-vet .
go run ./cmd/beach i18n --dir pkg --catalog pkg/i18n/catalog.json
```

A task is not complete until gen → vet → test → browser all pass.
