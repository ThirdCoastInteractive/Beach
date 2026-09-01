// Package chat is askmuskie's global, ToS-gated chat. Messages broadcast over
// the hub (so they ride the same SSE stream as the world frames), persist to
// Postgres for audit off the hot path, and bump the sender's chat stat. A small
// in-memory ring serves instant backlog to new connections so the DB isn't hit
// per connect at scale.
package chat

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
)

// Topic is the hub topic chat frames publish to.
const Topic = "chat"

const (
	maxLen     = 280
	ringSize   = 50
	rateWindow = 900 * time.Millisecond
)

// chatCounter is the (nil-safe) stats hook for chat-message counts.
type chatCounter interface{ Chat(accountID int64) }

// Msg is one chat line (compact JSON: name + body).
type Msg struct {
	N string `json:"n"`
	B string `json:"b"`
}

// Chat is the global chat service.
type Chat struct {
	hub   *hub.Hub
	pool  *pgxpool.Pool
	stats chatCounter

	mu   sync.Mutex
	ring []Msg
	last map[int64]time.Time // per-account rate limiter
}

// New builds the chat service over the hub + pool, with an optional stats hook.
func New(h *hub.Hub, pool *pgxpool.Pool, stats chatCounter) *Chat {
	return &Chat{hub: h, pool: pool, stats: stats, last: map[int64]time.Time{}}
}

// Post validates, broadcasts, persists, and counts a message. It returns false
// when dropped (empty, no account, or rate-limited).
func (c *Chat) Post(uid int64, name, body string) bool {
	body = strings.TrimSpace(body)
	if uid == 0 || body == "" {
		return false
	}
	if len(body) > maxLen {
		body = body[:maxLen]
	}
	now := time.Now()
	c.mu.Lock()
	if t, ok := c.last[uid]; ok && now.Sub(t) < rateWindow {
		c.mu.Unlock()
		return false
	}
	c.last[uid] = now
	c.ring = append(c.ring, Msg{N: name, B: body})
	if len(c.ring) > ringSize {
		c.ring = c.ring[len(c.ring)-ringSize:]
	}
	c.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = c.pool.Exec(ctx,
			`INSERT INTO askmuskie_chat (user_id, username, body) VALUES ($1, $2, $3)`, uid, name, body)
	}()
	if c.stats != nil {
		c.stats.Chat(uid)
	}

	if b, err := json.Marshal(map[string]any{"k": "chat", "n": name, "b": body}); err == nil {
		c.hub.Publish(Topic, hub.Event{Bytes: b})
	}
	return true
}

// BacklogFrame returns a {k:"chatlog"} frame of the recent ring, for a new
// connection's catch-up.
func (c *Chat) BacklogFrame() []byte {
	c.mu.Lock()
	msgs := append([]Msg(nil), c.ring...)
	c.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"k": "chatlog", "msgs": msgs})
	return b
}
