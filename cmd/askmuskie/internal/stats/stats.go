// Package stats is askmuskie's write-behind per-player stat recorder. The sim
// (and chat) call Wisdom/Fed/Chat with an account id; those are non-blocking
// sends onto a channel that a background goroutine batches into Postgres every
// couple of seconds, so the hot path never waits on the database. It satisfies
// pond.Recorder.
package stats

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type kind uint8

const (
	kWisdom kind = iota
	kFed
	kChat
)

type inc struct {
	uid int64
	k   kind
}

// counts accumulates per-account increments between flushes.
type counts struct{ wisdom, fed, chats int }

// Stats is the recorder. Construct with New and run Run in a goroutine.
type Stats struct {
	pool *pgxpool.Pool
	ch   chan inc
}

// New builds a recorder over the pool.
func New(pool *pgxpool.Pool) *Stats {
	return &Stats{pool: pool, ch: make(chan inc, 4096)}
}

// Wisdom/Fed/Chat record one increment for an account. They never block: a full
// buffer drops the increment (stats are best-effort).
func (s *Stats) Wisdom(accountID int64) { s.send(accountID, kWisdom) }
func (s *Stats) Fed(accountID int64)    { s.send(accountID, kFed) }
func (s *Stats) Chat(accountID int64)   { s.send(accountID, kChat) }

func (s *Stats) send(uid int64, k kind) {
	if uid == 0 {
		return
	}
	select {
	case s.ch <- inc{uid, k}:
	default:
	}
}

// Run drains increments, batching them into Postgres every flush interval until
// ctx is cancelled (a final flush runs on shutdown).
func (s *Stats) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	pending := map[int64]*counts{}
	for {
		select {
		case <-ctx.Done():
			s.flush(context.Background(), pending)
			return
		case in := <-s.ch:
			c := pending[in.uid]
			if c == nil {
				c = &counts{}
				pending[in.uid] = c
			}
			switch in.k {
			case kWisdom:
				c.wisdom++
			case kFed:
				c.fed++
			case kChat:
				c.chats++
			}
		case <-t.C:
			s.flush(ctx, pending)
			pending = map[int64]*counts{}
		}
	}
}

func (s *Stats) flush(ctx context.Context, pending map[int64]*counts) {
	for uid, c := range pending {
		_, _ = s.pool.Exec(ctx,
			`UPDATE askmuskie_players
			    SET wisdom = wisdom + $2, fed = fed + $3, chats = chats + $4, updated_at = now()
			  WHERE user_id = $1`,
			uid, c.wisdom, c.fed, c.chats)
	}
}
