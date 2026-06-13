// Live policy. The gate-and-moderation knobs an operator actually retunes —
// how many seats are open, what hides a post, how far an invite chain reaches,
// where standing unlocks things, who gets the admin mail — live in hush_settings
// (Postgres), editable at /admin/settings with no restart. The HUSH_* env vars
// seed the table on first boot; after that the database is authoritative.
//
// Everything else stays in env (config.go): ports, DSNs, secrets, mail
// transport, storage, and the scoring weights baked into the Repute engine at
// construction. The line is "how the gate behaves" (here) vs "how to reach the
// world and compute reputation" (there).
package main

import (
	"context"
	"strconv"
)

// liveSettings is the policy snapshot. It's held behind an atomic pointer on
// the app and read on the hot path (tierOf runs on every page), so reads never
// take a lock or touch the database; a save swaps the whole pointer.
type liveSettings struct {
	Tier1      float64
	Tier2      float64
	FlagTier   int
	FlagQuorum int
	InviteBase int
	InviteMax  int
	OpenSlots  int
	AdminEmail string
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

// settingsSeed is the boot default: the env-loaded config's policy values,
// used to seed an empty table and to backfill any key the table is missing.
func settingsSeed(cfg config) liveSettings {
	return liveSettings{
		Tier1:      cfg.Tier1,
		Tier2:      cfg.Tier2,
		FlagTier:   cfg.FlagTier,
		FlagQuorum: cfg.FlagQuorum,
		InviteBase: cfg.InviteBase,
		InviteMax:  cfg.InviteMax,
		OpenSlots:  cfg.OpenSlots,
		AdminEmail: cfg.AdminEmail,
	}
}

// ftoa/itoa format a setting for storage or for the admin form's value
// attribute — ftoa drops trailing zeros so "3" stays "3", not "3.000000".
func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
func itoa(n int) string     { return strconv.Itoa(n) }

// asRows flattens the policy to the key/value pairs hush_settings stores.
func (s liveSettings) asRows() map[string]string {
	return map[string]string{
		keyTier1:      ftoa(s.Tier1),
		keyTier2:      ftoa(s.Tier2),
		keyFlagTier:   itoa(s.FlagTier),
		keyFlagQuorum: itoa(s.FlagQuorum),
		keyInviteBase: itoa(s.InviteBase),
		keyInviteMax:  itoa(s.InviteMax),
		keyOpenSlots:  itoa(s.OpenSlots),
		keyAdminEmail: s.AdminEmail,
	}
}

// applyRow overlays one stored key, keeping the existing (seed) value if the
// stored text won't parse — a hand-edited bad row degrades, never panics.
func (s *liveSettings) applyRow(key, val string) {
	switch key {
	case keyTier1:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			s.Tier1 = f
		}
	case keyTier2:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			s.Tier2 = f
		}
	case keyFlagTier:
		if n, err := strconv.Atoi(val); err == nil {
			s.FlagTier = n
		}
	case keyFlagQuorum:
		if n, err := strconv.Atoi(val); err == nil {
			s.FlagQuorum = n
		}
	case keyInviteBase:
		if n, err := strconv.Atoi(val); err == nil {
			s.InviteBase = n
		}
	case keyInviteMax:
		if n, err := strconv.Atoi(val); err == nil {
			s.InviteMax = n
		}
	case keyOpenSlots:
		if n, err := strconv.Atoi(val); err == nil {
			s.OpenSlots = n
		}
	case keyAdminEmail:
		s.AdminEmail = val
	}
}

// live returns the current policy, lock-free. Always safe: the seed pointer is
// stored at boot before any handler runs, in database mode and out of it.
func (a *app) live() liveSettings {
	return *a.settings.Load()
}

// loadSettings seeds hush_settings from the env defaults (missing keys only),
// then reads the table back over the seed and publishes it. After first boot
// the table wins; env just fills gaps a new release might add.
func (a *app) loadSettings(ctx context.Context, seed liveSettings) error {
	for k, v := range seed.asRows() {
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO hush_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`, k, v); err != nil {
			return err
		}
	}
	return a.refreshSettings(ctx, seed)
}

// refreshSettings reads the table over a fresh seed and swaps the live pointer.
func (a *app) refreshSettings(ctx context.Context, seed liveSettings) error {
	ls := seed
	rows, err := a.pool.Query(ctx, `SELECT key, value FROM hush_settings`)
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
	a.settings.Store(&ls)
	return nil
}

// saveSettings upserts the whole policy and publishes it — the admin edit takes
// effect on the next request, no restart.
func (a *app) saveSettings(ctx context.Context, ls liveSettings) error {
	for k, v := range ls.asRows() {
		if _, err := a.pool.Exec(ctx,
			`INSERT INTO hush_settings (key, value, updated_at) VALUES ($1, $2, now())
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, k, v); err != nil {
			return err
		}
	}
	a.settings.Store(&ls)
	return nil
}
