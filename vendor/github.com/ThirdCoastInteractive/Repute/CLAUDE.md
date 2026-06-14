# CLAUDE.md

Orientation for agents working in the Repute repo. Read this before editing.

## What this is

Repute is a **replayable, scoring-rule-pluggable reputation engine** in Go.
Reputation is a *projection* over an immutable, append-only event log — never
stored mutable state. You feed the engine events; installed scorers emit deltas
on named tracks; current scores are the decayed fold of those deltas at a
caller-supplied as-of time.

The one-sentence mental model: **an event-sourcing projection engine with
pluggable reducers (scorers) and time-decay, where reputation is the first
vertical.** The kernel knows nothing about debates or any domain.

## The invariants that must never break

These are the whole value proposition. A change that violates one is wrong even
if it compiles and passes tests:

1. **Determinism.** Same event log + same ruleset ⇒ identical scores, every
   time, on any machine. This is what lets a leaderboard be independently
   re-derived and verified.
2. **Purity of scorers.** A `Scorer.Score` must NOT read the wall clock
   (`time.Now`), call out to I/O, use randomness, or touch global mutable state.
   The only clock a scorer may read is `view.AsOf()` / `event.Time` — domain
   time, which travels inside the event. Stateful scorers (Glicko-2, TrueSkill)
   may cache internal state but it must be a deterministic function of the events
   seen, in `Seq` order, and they must implement `Resettable`.
3. **Append-only, never mutate.** Events and deltas are immutable once created.
   Scores are derived, never written directly. "Correcting" history means
   appending a compensating event, not editing a past one.
4. **The kernel stays domain-agnostic.** No debate-specific (or any
   domain-specific) logic in the root package. Domain meaning lives in scorers
   (`scorers/`) and in the caller's event payloads.
5. **Adapters don't leak into the kernel.** The root `repute` package imports no
   database driver, no transport, nothing under `eventsource/`. Dependencies flow
   inward: adapters depend on the kernel, never the reverse.

If a task seems to require breaking one of these, stop and flag it — there's
almost always a design-preserving alternative, and the invariant is more valuable
than the feature.

## Repo layout

```
.
├── doc.go            Package overview (read this for the conceptual frame)
├── event.go          Event: the append-only log record. Seq, Time, Kind, Actors, Payload.
├── delta.go          Delta + ScoreMode (Add accumulates, Set replaces). The unit scorers emit.
├── scorer.go         Scorer + View + Resettable interfaces. The plug-in contract.
├── decay.go          Decay interface + 8 models + DecayModel registry. Applied at projection time.
├── state.go          State: the materialized projection (additive deltas + Set ratings) + Leaderboard.
├── engine.go         Engine: Apply / Replay / Score / Leaderboard / Snapshot / RulesetID. The core.
├── snapshot.go       Serialize/restore the projection (NOT stateful-scorer caches — see caveat).
├── scorers/          All Scorer implementations. Opt-in, independent of each other.
│   ├── scorers.go    Shared payloads (match, verdict, btsBallot) + helpers (clamp01).
│   ├── elo.go        Elo (Set mode).
│   ├── glicko2.go    Glicko-2 (Set, Resettable, internal RD/volatility). Preferred for async play.
│   ├── trueskill.go  TrueSkill 1v1 (Set, Resettable, internal mu/sigma).
│   ├── properscore.go Brier / Logarithmic / Spherical (Add). Judge calibration.
│   ├── bts.go        BTS + SurprisinglyPopular (Add). Anti-conformity / no-ground-truth.
│   └── extra.go      Participation, ConsensusCalibration, WeightedWin (Add).
├── eventsource/
│   ├── memory/       In-memory log + replay source. Used by tests/examples.
│   ├── pg/           Postgres over database/sql, sqlc-friendly. schema.sql + queries.sql. The AUTHORITATIVE log.
│   └── clickhouse/   High-volume mirror for analytical replays. Mirrors pg's seq.
├── cmd/repute/       CLI: `replay` and `verify` over a JSONL log. The trust/demo surface.
├── examples/debate/  End-to-end wiring showing how masterdebaters.net uses it.
└── docs/             Design specs and handoff docs.
```

## How the pieces fit (one pass)

- A caller creates `Event`s and appends them to a log (a `Log`/`Store`), which
  assigns the monotonic `Seq`.
- `Engine.Apply(ev)` runs every `Scorer` whose `Scorable(ev.Kind)` is true, in
  install order, each producing `[]Delta`. The engine stamps `SourceSeq`+`At`
  on each delta and folds it into `State`.
- `State` keeps, per `(actor, track)`: a list of **Add** deltas (decayed lazily
  at read time) and the latest **Set** rating (not decayed by the engine).
- `Engine.Score(actor, track, asOf)` and `Engine.Leaderboard(track, asOf)`
  compute the decayed projection *as of* the given time.
- `Engine.Replay(ctx, src)` rebuilds `State` from scratch over an
  `EventSource`, resetting `Resettable` scorers first. This is the determinism
  workhorse.
- Scorer install order matters: a scorer sees the `View` as mutated by earlier
  scorers on the *same* event (the log is totally ordered, so "before" is
  well-defined). This is intentional and documented.

## Add-mode vs Set-mode (a common confusion)

- **Add**: the track value is the decayed *sum* of all Add deltas. Used by
  scoring rules, BTS, participation. These deltas ARE subject to the engine's
  decay model (via `Delta.Decay` naming a registered decay, or the default).
- **Set**: the track value *becomes* `Amount`. Used by rating systems that
  compute the new absolute rating internally (Elo, Glicko-2, TrueSkill). Set
  deltas are NOT decayed by the engine — if a rating system wants
  inactivity-decay it bakes it into the emitted rating itself.

Don't mix modes on one track unless you mean to. A track that gets both Add and
Set deltas sums the Set rating with the decayed Add deltas, which is rarely what
you want.

## Conventions

- **Go 1.23+.** Standard library only in the kernel and scorers. Adapters use
  `database/sql` (no concrete driver imported — the caller injects one).
- **Formatting/quality gates, all must pass before considering work done:**
  ```
  gofmt -l .        # must print nothing
  go vet ./...      # must be clean
  go build ./...
  go test -race ./...
  ```
- **Doc comments** on every exported identifier, explaining not just what but
  *why* where the reasoning is non-obvious (the existing code sets the bar —
  match its density and tone). The comments carry the design rationale; keep
  them accurate when you change behavior.
- **Tests are part of the contract,** not an afterthought. Any new scorer needs:
  a behavior test (does it reward the right thing?) and inclusion in a
  determinism test (replay twice, assert identical). Any kernel change needs a
  test that would fail if an invariant above broke. See
  `engine_test.go` and `scorers/scorers_test.go` for the patterns —
  `resolve()` and the `memory.NewLog()` + `Apply` loop are the idioms.
- **No new dependencies** without explicit sign-off. The zero-dependency kernel
  is a feature.
- **Naming:** scorers are nouns describing the method (`Glicko2`, `ProperScore`);
  tracks are lowercase domain roles (`"debater"`, `"judge"`). Event kinds are
  dotted (`"debate.resolved"`, `"verdict.scored"`).

## Things that will bite you

- **Snapshots don't capture stateful-scorer internal state.** `Engine.Snapshot`
  serializes the `State` projection (Add deltas + Set ratings) but NOT a
  Glicko-2 scorer's cached RDs or TrueSkill's sigmas. After `LoadSnapshot`, those
  scorers think every actor is new. For tracks using stateful scorers you must
  replay from the start, or extend those scorers to persist their own state.
  This is documented in `snapshot.go`; don't "fix" it without understanding why
  the projection and the scorer-cache are deliberately separate.
- **`Seq` must be ascending in `Apply`/`Replay`,** but NOT necessarily
  contiguous. Gaps are legal (this matters for the counterfactual feature, which
  drops events). Don't add a contiguity check.
- **`Event.Time` must be non-decreasing with `Seq`** for decay to behave
  monotonically. Producers are responsible for this; the kernel assumes it.
- **`Payload` is `json.RawMessage`** — exact bytes as stored. Don't re-marshal it
  in code paths that need byte-stability (this is critical for the planned
  hash-chaining feature). Re-marshaling reorders keys and changes bytes.
- **Floating-point determinism:** scores are `float64`. Determinism holds because
  the *same operations in the same order* produce the same bits. Don't introduce
  map-iteration-order-dependent accumulation (Go randomizes map order). Where you
  must iterate a map to accumulate, sort keys first — see `state.go`'s
  leaderboard sort and the `RulesetID` decay-name sort for the pattern.

## Current work

See `docs/SPEC-hashchain-counterfactual.md` for the two features in flight:
tamper-evident hash-chaining and counterfactual replay. Both build on the
existing kernel without changing the scorer interface. Implement hash-chaining
first; canonicalization is the subtle part and gets its own tests.

## What's deliberately out of scope

Sybil/identity resistance, vote ingestion and eligibility, transport/HTTP/SSE,
and persistence of the caller's domain objects. Repute scores an event log; it
does not run the contest. If a task drifts into these, it probably belongs in the
application (e.g. masterdebaters.net), not here — flag it.

## Owner

Third Coast Interactive LLC. License: MIT.
