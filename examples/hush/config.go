package main

import (
	"os"
	"strconv"
)

// config is hush's env-driven knobs, read once at boot. Everything tunable —
// tier thresholds, moderation triggers, the signup cap — lives here so the
// rest of the app reads a.cfg instead of os.Getenv, and a deploy can retune
// the wall without a rebuild.
type config struct {
	// Tier1 is the author-track score that unlocks tier 1 (the profile).
	// HUSH_TIER1, default 3.
	Tier1 float64

	// Tier2 is the score that unlocks tier 2 (DMs). HUSH_TIER2, default 8.
	Tier2 float64

	// FlagTier is the tier whose single flag insta-hides a post — trusted
	// members moderate alone. HUSH_FLAG_TIER, default 1.
	FlagTier int

	// FlagQuorum is how many tier-0 flags it takes to hide a post — the
	// untrusted crowd moderates by agreement. HUSH_FLAG_QUORUM, default 3.
	FlagQuorum int

	// EmailPoints is the author-track reputation granted for verifying an
	// email address. HUSH_EMAIL_POINTS, default 2.
	EmailPoints float64

	// InviteBase is a tier-1 member's starting invite allowance; the chain
	// score moves it from there (see inviteAllowance). HUSH_INVITE_BASE,
	// default 3.
	InviteBase int

	// InviteMax caps the allowance however beloved the chain — nobody mints
	// the whole site. HUSH_INVITE_MAX, default 6.
	InviteMax int

	// OpenSlots opens the first N registrations with no review (the
	// switch-on-later mode); once spent, the application queue resumes.
	// HUSH_OPEN_SLOTS, default 0 (queue + invites only).
	OpenSlots int

	// AdminEmail is where moderation notices go. HUSH_ADMIN_EMAIL, no default.
	AdminEmail string

	// BaseURL is the site's absolute origin, used to build absolute links in
	// outgoing emails. HUSH_BASE_URL, default http://localhost:8080.
	BaseURL string
}

// loadConfig reads the HUSH_* environment into a config, falling back to the
// documented defaults on unset or unparseable values.
func loadConfig() config {
	return config{
		Tier1:       envFloat("HUSH_TIER1", 3),
		Tier2:       envFloat("HUSH_TIER2", 8),
		FlagTier:    envInt("HUSH_FLAG_TIER", 1),
		FlagQuorum:  envInt("HUSH_FLAG_QUORUM", 3),
		EmailPoints: envFloat("HUSH_EMAIL_POINTS", 2),
		InviteBase:  envInt("HUSH_INVITE_BASE", 3),
		InviteMax:   envInt("HUSH_INVITE_MAX", 6),
		OpenSlots:   envInt("HUSH_OPEN_SLOTS", 0),
		AdminEmail:  os.Getenv("HUSH_ADMIN_EMAIL"),
		BaseURL:     envStr("HUSH_BASE_URL", "http://localhost:8080"),
	}
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
