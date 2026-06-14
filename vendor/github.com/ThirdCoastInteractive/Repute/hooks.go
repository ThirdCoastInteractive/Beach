package repute

import "time"

// Hooks provides optional observability callbacks for engine operations. All
// fields are optional; nil callbacks are never invoked. Hooks fire
// synchronously from the engine goroutine — keep them fast or hand off to a
// channel if you need async processing.
//
// Wire with WithHooks. Compose multiple hook sets with CombineHooks.
type Hooks struct {
	// OnApply fires after each event is folded and its deltas ingested.
	OnApply func(ApplyInfo)

	// OnReplay fires once when a full Replay completes.
	OnReplay func(ReplayInfo)

	// OnScore fires when Score computes a single (actor, track) value.
	OnScore func(ScoreInfo)

	// OnLeaderboard fires when a leaderboard is materialized.
	OnLeaderboard func(LeaderboardInfo)
}

// ApplyInfo describes a single Apply call.
type ApplyInfo struct {
	Event    Event
	Deltas   []Delta
	Duration time.Duration
}

// ReplayInfo describes a completed Replay.
type ReplayInfo struct {
	EventCount int
	LastSeq    uint64
	Duration   time.Duration
}

// ScoreInfo describes a single score computation.
type ScoreInfo struct {
	Actor    ActorID
	Track    Track
	Value    float64
	AsOf     time.Time
	Duration time.Duration
}

// LeaderboardInfo describes a leaderboard computation.
type LeaderboardInfo struct {
	Track    Track
	Entries  int
	AsOf     time.Time
	Duration time.Duration
}

// WithHooks installs observability hooks on the engine.
func WithHooks(h *Hooks) Option { return func(e *Engine) { e.hooks = h } }

// CombineHooks merges multiple Hooks into one that calls each in order.
// Nil entries are safely skipped.
func CombineHooks(hooks ...*Hooks) *Hooks {
	return &Hooks{
		OnApply: func(info ApplyInfo) {
			for _, h := range hooks {
				if h != nil && h.OnApply != nil {
					h.OnApply(info)
				}
			}
		},
		OnReplay: func(info ReplayInfo) {
			for _, h := range hooks {
				if h != nil && h.OnReplay != nil {
					h.OnReplay(info)
				}
			}
		},
		OnScore: func(info ScoreInfo) {
			for _, h := range hooks {
				if h != nil && h.OnScore != nil {
					h.OnScore(info)
				}
			}
		},
		OnLeaderboard: func(info LeaderboardInfo) {
			for _, h := range hooks {
				if h != nil && h.OnLeaderboard != nil {
					h.OnLeaderboard(info)
				}
			}
		},
	}
}
