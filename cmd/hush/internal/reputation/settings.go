// Live policy. The gate-and-moderation knobs an operator actually retunes —
// how many seats are open, what hides a post, how far an invite chain reaches,
// where standing unlocks things, who gets the admin mail — live in hush_settings
// (Postgres), editable at /admin/settings with no restart. The HUSH_* env vars
// seed the table on first boot; after that the database is authoritative.
//
// Everything else stays in env (the web layer's config): ports, DSNs, secrets,
// mail transport, storage, and the scoring weights baked into the Repute engine
// at construction. The line is "how the gate behaves" (here) vs "how to reach
// the world and compute reputation" (there).
package reputation

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Live is the policy snapshot. It's held behind an atomic pointer on Settings
// and read on the hot path (TierOf runs on every page), so reads never take a
// lock or touch the database; a save swaps the whole pointer.
type Live struct {
	Tier1      float64
	Tier2      float64
	FlagTier   int
	FlagQuorum int
	InviteBase int
	InviteMax  int
	OpenSlots  int
	AdminEmail string
}

// Settings is the Postgres-backed live policy: the authoritative table plus the
// atomic snapshot the hot path reads.
type Settings struct {
	pool *pgxpool.Pool
	cur  atomic.Pointer[Live]
}

// NewSettings publishes the env-seeded policy before anything serves, so Live()
// is safe everywhere; LoadSettings reseeds + loads the authoritative table.
func NewSettings(pool *pgxpool.Pool, seed Live) *Settings {
	s := &Settings{pool: pool}
	s.cur.Store(&seed)
	return s
}

// the persisted keys, kept in one place so seed, load, save, and the admin
// form never drift apart.
const (
	keyTier1      = "tier1"
	keyTier2      = "tier2"
	keyFlagTier   = "flag_tier"
	keyFlagQuorum = "flag_quorum"
	keyInviteBase = "invite_base"
	keyInviteMax  = "invite_max"
	keyOpenSlots  = "open_slots"
	keyAdminEmail = "admin_email"
)

// ftoa/itoa format a setting for storage or for the admin form's value
// attribute — ftoa drops trailing zeros so "3" stays "3", not "3.000000".
func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
func itoa(n int) string     { return strconv.Itoa(n) }

// asRows flattens the policy to the key/value pairs hush_settings stores.
func (l Live) asRows() map[string]string {
	return map[string]string{
		keyTier1:      ftoa(l.Tier1),
		keyTier2:      ftoa(l.Tier2),
		keyFlagTier:   itoa(l.FlagTier),
		keyFlagQuorum: itoa(l.FlagQuorum),
		keyInviteBase: itoa(l.InviteBase),
		keyInviteMax:  itoa(l.InviteMax),
		keyOpenSlots:  itoa(l.OpenSlots),
		keyAdminEmail: l.AdminEmail,
	}
}

// applyRow overlays one stored key, keeping the existing (seed) value if the
// stored text won't parse — a hand-edited bad row degrades, never panics.
func (l *Live) applyRow(key, val string) {
	switch key {
	case keyTier1:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			l.Tier1 = f
		}
	case keyTier2:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			l.Tier2 = f
		}
	case keyFlagTier:
		if n, err := strconv.Atoi(val); err == nil {
			l.FlagTier = n
		}
	case keyFlagQuorum:
		if n, err := strconv.Atoi(val); err == nil {
			l.FlagQuorum = n
		}
	case keyInviteBase:
		if n, err := strconv.Atoi(val); err == nil {
			l.InviteBase = n
		}
	case keyInviteMax:
		if n, err := strconv.Atoi(val); err == nil {
			l.InviteMax = n
		}
	case keyOpenSlots:
		if n, err := strconv.Atoi(val); err == nil {
			l.OpenSlots = n
		}
	case keyAdminEmail:
		l.AdminEmail = val
	}
}

// Live returns the current policy, lock-free. Always safe: the seed pointer is
// stored at construction before any handler runs.
func (s *Settings) Live() Live {
	return *s.cur.Load()
}

// Load seeds hush_settings from the env defaults (missing keys only), then reads
// the table back over the seed and publishes it. After first boot the table
// wins; env just fills gaps a new release might add.
func (s *Settings) Load(ctx context.Context, seed Live) error {
	for k, v := range seed.asRows() {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO hush_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`, k, v); err != nil {
			return err
		}
	}
	return s.Refresh(ctx, seed)
}

// Refresh reads the table over a fresh seed and swaps the live pointer.
func (s *Settings) Refresh(ctx context.Context, seed Live) error {
	ls := seed
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM hush_settings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		ls.applyRow(k, v)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.cur.Store(&ls)
	return nil
}

// Save upserts the whole policy and publishes it — the admin edit takes effect
// on the next request, no restart.
func (s *Settings) Save(ctx context.Context, ls Live) error {
	for k, v := range ls.asRows() {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO hush_settings (key, value, updated_at) VALUES ($1, $2, now())
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, k, v); err != nil {
			return err
		}
	}
	s.cur.Store(&ls)
	return nil
}
