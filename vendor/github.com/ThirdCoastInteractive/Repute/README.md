# Repute

A replayable, scoring-rule-pluggable reputation engine in Go.

Repute turns an append-only log of events into reputation. Current scores are a
**projection** over that log — never a stored, mutable fact — which makes the
whole system auditable, replayable, and independently verifiable. Change the
rules, replay the log, get new scores. Publish the log and anyone can re-derive
your leaderboard and confirm it wasn't tampered with.

Built for [masterdebaters.net](https://masterdebaters.net), but the kernel knows
nothing about debates. It scores an event stream; it does not run your contest.

## Design principles

1. **Reputation is a projection, not a stored fact.** Score = decayed fold over
   an immutable event log. Never mutate a score directly.
2. **Deterministic and pure.** Same log + same ruleset ⇒ identical scores,
   byte-for-byte. Scorers never read the wall clock; domain time lives *in* the
   event. This is what makes a leaderboard re-derivable.
3. **The kernel knows nothing about your domain.** Events are opaque-typed;
   meaning lives in the scorers you install.
4. **Sybil resistance is out of scope.** No scoring math survives free account
   minting — account-gating and rate limits do that work, in your app.
5. **Multi-track by default.** Reputation is a vector (debater, judge, …), not a
   scalar. Different competencies stay orthogonal.

## Install

```
go get github.com/ThirdCoastInteractive/Repute
```

## Quick start

```go
eng := repute.New([]repute.Scorer{
    &scorers.Glicko2{Track: "debater", Kinds: []string{"debate.resolved"}},
    scorers.ProperScore{Track: "judge", Rule: scorers.Brier, Kinds: []string{"verdict.scored"}},
}, repute.WithDecay(repute.NewDecayModel(
    repute.Exponential{HalfLife: 90 * 24 * time.Hour},
)))

ev, _ := repute.NewEvent(time.Now(), "debate.resolved",
    map[string]any{"a": "alice", "b": "bob", "score": 1.0})
eng.Apply(ev)

board := eng.Leaderboard("debater", time.Now())
```

See [`examples/debate`](./examples/debate) for a full wiring.

## Scorers

Each is an independent, opt-in `repute.Scorer`.

| Scorer | Track kind | What it rewards |
|---|---|---|
| `Elo` | head-to-head | Familiar chess-style rating. Simple, no uncertainty model. |
| `Glicko2` | head-to-head | Rating + deviation + volatility. **Best for irregular async play.** Cold-start and inactivity handled natively. |
| `TrueSkill` | head-to-head | Bayesian Gaussian skill (mu, sigma). Fast convergence, draw support. |
| `WeightedWin` | head-to-head | Transparent decaying points by margin. No rating system. |
| `ProperScore` | judge | Calibration via Brier / Logarithmic / Spherical rules. Confident-correct pays; confident-wrong costs; hedging earns ~0. |
| `BTS` | judge | Bayesian Truth Serum. Rewards the **surprisingly popular** answer — provably truth-inducing, the antidote to conformity. |
| `SurprisinglyPopular` | judge | Lighter SP decision rule as a reward. |
| `ConsensusCalibration` | judge | Rewards being correct **early**, not agreeing **late**. Contrarian-correct beats piling onto obvious consensus. |
| `Participation` | participation | Small fast-decaying cold-start reward for showing up. |

### Why not just "reward agreeing with consensus"?

Because that breeds conformity cascades: people learn to vote for who they
*predict will win*, not who they think won, and the vote signal degrades into a
beauty contest. `ProperScore`, `BTS`, and `ConsensusCalibration` reward being
**calibrated and early against the eventual verdict** instead — same spirit,
without the sycophancy.

## Decay models

Decay is applied at projection time, so the same log yields different "current"
standings at different as-of times while staying deterministic for any fixed
as-of.

`NoDecay`, `Exponential{HalfLife}`, `Linear{Window}`, `Step{After,Residual}`,
`PowerLaw{Scale,Exponent}`, `Gaussian{Sigma}`, `Logistic{Midpoint,Steepness}`,
and `Floored{Inner,Floor}` (wrap any model with a permanent residual).

## Storage adapters

The kernel has zero DB dependencies. Adapters are separate subpackages:

- **`eventsource/memory`** — in-memory log + replay source. Tests, examples,
  single-process use.
- **`eventsource/pg`** — Postgres over `database/sql`, sqlc-friendly. Ships
  `schema.sql` + `queries.sql` and an audit-trail table so every score is
  explainable. This is the authoritative log.
- **`eventsource/clickhouse`** — high-volume firehose mirror for fast analytical
  replays; mirrors the authoritative Postgres `seq` for a consistent order.

## Snapshots

`Engine.Snapshot()` / `LoadSnapshot()` serialize the additive/Set projection so
you can restart without refolding all history, then replay only events after the
snapshot's `LastSeq`. Note: stateful scorers (Glicko-2 RDs, TrueSkill sigmas)
are not captured by snapshots and need a full replay or their own persistence.

## Verifiability

Every `Delta` carries a human-readable `Reason` and the `SourceSeq` of the event
that produced it, so any score decomposes into the exact events behind it. The
`cmd/repute` CLI replays a public JSONL log and `verify`s a published
leaderboard against a fresh recomputation:

```
repute replay -log events.jsonl -track debater
repute verify -log events.jsonl -expected board.json -track debater
```

The installed scorers + decay names hash to a `RulesetID`. Publish leaderboards
as `(RulesetID, as-of, scores)` so any change to scoring is a visible, dated
event rather than a silent edit.

## What's out of scope

Identity/Sybil resistance, vote ingestion and eligibility, transport/API, and
persistence of your domain objects. Repute scores an event log. Everything else
is your application's job.

## License

MIT © Third Coast Interactive LLC
