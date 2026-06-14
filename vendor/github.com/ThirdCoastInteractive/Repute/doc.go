// Package repute is a replayable, scoring-rule-pluggable reputation engine.
//
// Reputation is modeled as a projection over an immutable, append-only event
// log rather than as stored mutable state. An Engine folds events through a set
// of installed Scorers, each emitting Deltas on named Tracks; current scores are
// the decayed sum of those deltas, evaluated at a caller-supplied as-of time.
//
// The core guarantees are determinism and replayability: the same event log and
// the same ruleset always produce identical scores, because scorers are pure and
// never consult the wall clock (domain time travels inside each Event). This is
// what lets a leaderboard be independently re-derived and verified from a public
// log — the trust property the package is built around.
//
// The kernel is domain-agnostic. Ready-made scorers (Elo, Glicko-2, TrueSkill,
// proper scoring rules, Bayesian Truth Serum, consensus calibration,
// participation) live in package scorers and are entirely opt-in. Storage
// adapters for in-memory, Postgres, and ClickHouse live under eventsource and
// depend on nothing in the kernel.
//
// Sybil resistance, identity, vote eligibility, transport, and persistence of
// domain objects are explicitly out of scope: Repute scores an event log, it
// does not run your contest.
package repute
