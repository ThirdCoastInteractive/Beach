// Package store is booking-manager's Postgres-backed system of record:
// properties and their key codes, guest inquiries and bookings, the staffing
// suite (applicants, staff, shifts, time entries), and supply inventory.
// booking-manager requires a database, so this is the only implementation (no
// in-memory fallback). Queries are hand-written pgx with explicit column
// lists; the method surface stays narrow so a generated db package could slot
// in mechanically.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Postgres-backed booking store.
type Store struct {
	pool *pgxpool.Pool
}

// New returns the store and idempotently seeds a fresh database with a small
// working operation (3 properties with codes and supplies, a cleaner on
// staff, applicants in the pipeline, an open inquiry, and two bookings) so
// every page renders something on first run. Safe to run on every boot: it
// only inserts when bm_properties is empty.
func New(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	s := &Store{pool: pool}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) seed(ctx context.Context) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM bm_properties`).Scan(&n); err != nil {
		return fmt.Errorf("booking: seed count: %w", err)
	}
	if n > 0 {
		return nil
	}

	// Properties first; capture ids to hang everything else off.
	var loon, birch, boat int64
	for _, p := range []struct {
		name, desc, addr, lock string
		sleeps, beds           int
		rate                   int64
		id                     *int64
	}{
		{"Loon Lake Cottage", "Two-bedroom lakefront cottage with a dock and firepit.",
			"41 Loon Lake Rd", "lock-loon-front", 6, 2, 21500, &loon},
		{"Birch Hollow Cabin", "Off-grid-feel cabin on ten wooded acres, hot tub on the deck.",
			"188 Birch Hollow Ln", "lock-birch-front", 4, 2, 17500, &birch},
		{"The Boathouse", "Converted boathouse studio right on the water. Sleeps two, sunsets included.",
			"7 Harbor Row", "", 2, 1, 14000, &boat},
	} {
		if err := s.pool.QueryRow(ctx,
			`INSERT INTO bm_properties (name, description, address, sleeps, bedrooms, nightly_rate_cents, lock_device_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			p.name, p.desc, p.addr, p.sleeps, p.beds, p.rate, p.lock).Scan(p.id); err != nil {
			return fmt.Errorf("booking: seed property %q: %w", p.name, err)
		}
	}

	// Standing key codes.
	for _, k := range []struct {
		prop         int64
		label, code  string
	}{
		{loon, "Lockbox", "4417"},
		{loon, "Boat shed", "0262"},
		{birch, "Gate", "8830"},
		{boat, "Lockbox", "5151"},
	} {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO bm_key_codes (property_id, label, code) VALUES ($1, $2, $3)`,
			k.prop, k.label, k.code); err != nil {
			return fmt.Errorf("booking: seed key code %q: %w", k.label, err)
		}
	}

	// Supplies with par levels; the paper towels run low on purpose so the
	// dashboard has a low-stock line on first boot.
	for _, sp := range []struct {
		prop      int64
		name, cat string
		qty, par  float64
		unit      string
	}{
		{loon, "Coffee (ground)", "kitchen", 3, 2, "bag"},
		{loon, "Paper towels", "kitchen", 1, 4, "roll"},
		{loon, "All-purpose cleaner", "cleaning", 2, 1, "bottle"},
		{loon, "Bath towels", "linens", 10, 8, "ea"},
		{birch, "Dish soap", "kitchen", 2, 1, "bottle"},
		{birch, "Firewood", "other", 12, 20, "bundle"},
		{birch, "Queen sheet sets", "linens", 4, 4, "set"},
		{boat, "Trash bags", "cleaning", 15, 10, "ea"},
		{boat, "Coffee pods", "kitchen", 8, 12, "ea"},
	} {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO bm_supplies (property_id, name, category, qty, par, unit)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			sp.prop, sp.name, sp.cat, sp.qty, sp.par, sp.unit); err != nil {
			return fmt.Errorf("booking: seed supply %q: %w", sp.name, err)
		}
	}

	// One cleaner on staff, two applicants still in the pipeline.
	var cleaner int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO bm_staff (name, email, phone, role, hourly_rate_cents)
		 VALUES ('Marta Kowalski', 'marta@example.com', '+15550002222', 'cleaner', 2200) RETURNING id`).Scan(&cleaner); err != nil {
		return fmt.Errorf("booking: seed staff: %w", err)
	}
	for _, a := range []struct {
		name, email, role, stage string
	}{
		{"Dev Patel", "dev@example.com", "cleaner", "interview"},
		{"June Osei", "june@example.com", "maintenance", "applied"},
	} {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO bm_applicants (name, email, role, stage) VALUES ($1, $2, $3, $4)`,
			a.name, a.email, a.role, a.stage); err != nil {
			return fmt.Errorf("booking: seed applicant %q: %w", a.name, err)
		}
	}

	// Bookings around "now" so the calendar and dashboard render live content:
	// a confirmed stay starting in 3 days and a pending one next week. Date
	// columns get ISO strings, never time.Time (see bookings.go isoDate).
	today := time.Now()
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format("2006-01-02") }
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO bm_bookings (property_id, guest_name, guest_email, guest_phone, check_in, check_out, status, rate_cents, door_code)
		 VALUES ($1, 'Ana Whitfield', 'ana@example.com', '+15550003333', $2, $3, 'confirmed', 21500, '4821')`,
		loon, day(3), day(6)); err != nil {
		return fmt.Errorf("booking: seed booking: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO bm_bookings (property_id, guest_name, guest_email, check_in, check_out, status, rate_cents)
		 VALUES ($1, 'Tom Reyes', 'tom@example.com', $2, $3, 'pending', 17500)`,
		birch, day(9), day(12)); err != nil {
		return fmt.Errorf("booking: seed booking: %w", err)
	}

	// A turnover shift for the confirmed arrival, plus one open inquiry.
	morning := time.Date(today.Year(), today.Month(), today.Day(), 10, 0, 0, 0, time.Local).AddDate(0, 0, 3)
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO bm_shifts (staff_id, property_id, starts_at, ends_at, kind, notes)
		 VALUES ($1, $2, $3, $4, 'turnover', 'Full clean before the Whitfield stay')`,
		cleaner, loon, morning, morning.Add(4*time.Hour)); err != nil {
		return fmt.Errorf("booking: seed shift: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO bm_inquiries (property_id, name, email, phone, party_size, check_in, check_out, message)
		 VALUES ($1, 'Priya Raman', 'priya@example.com', '+15550004444', 2, $2, $3,
		         'Is the boathouse available over the long weekend? We have a small dog.')`,
		boat, day(16), day(19)); err != nil {
		return fmt.Errorf("booking: seed inquiry: %w", err)
	}
	return nil
}
