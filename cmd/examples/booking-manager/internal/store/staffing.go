package store

import (
	"context"
	"time"
)

// Applicant is one candidate in the hiring pipeline
// applied -> interview -> offer -> hired|rejected.
type Applicant struct {
	ID        int64
	Name      string
	Email     string
	Phone     string
	Role      string
	Stage     string
	Notes     string
	CreatedAt time.Time
}

// Staff is one person on the payroll. OnClockSince is the open time entry's
// clock-in when they're currently working (nil = off the clock).
type Staff struct {
	ID              int64
	Name            string
	Email           string
	Phone           string
	Role            string
	HourlyRateCents int64
	Active          bool
	OnClockSince    *time.Time
}

// Shift is one scheduled block of work, staff and property names attached.
type Shift struct {
	ID         int64
	StaffID    int64
	Staff      string
	PropertyID int64 // 0 = general work
	Property   string
	StartsAt   time.Time
	EndsAt     time.Time
	Kind       string
	Notes      string
}

// StaffHours is one staffer's clocked total over a window.
type StaffHours struct {
	StaffID int64
	Name    string
	Hours   float64
	OnClock bool
}

// --- hiring ---

// ListApplicants returns the pipeline, open stages first, newest first within
// a stage.
func (s *Store) ListApplicants(ctx context.Context) ([]Applicant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, email, phone, role, stage, notes, created_at
		  FROM bm_applicants
		 ORDER BY (stage IN ('hired', 'rejected')), created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Applicant{}
	for rows.Next() {
		var a Applicant
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Phone, &a.Role, &a.Stage, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddApplicant inserts one candidate at the applied stage.
func (s *Store) AddApplicant(ctx context.Context, a Applicant) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_applicants (name, email, phone, role, notes)
		VALUES ($1, $2, $3, $4, $5)`,
		a.Name, a.Email, a.Phone, a.Role, a.Notes)
	return err
}

// SetApplicantStage moves a candidate and returns the updated row, which the
// hire handler needs to create the staff record.
func (s *Store) SetApplicantStage(ctx context.Context, id int64, stage string) (Applicant, error) {
	var a Applicant
	err := s.pool.QueryRow(ctx, `
		UPDATE bm_applicants SET stage = $2 WHERE id = $1
		RETURNING id, name, email, phone, role, stage, notes, created_at`, id, stage).
		Scan(&a.ID, &a.Name, &a.Email, &a.Phone, &a.Role, &a.Stage, &a.Notes, &a.CreatedAt)
	return a, err
}

// --- staff & time clock ---

// ListStaff returns everyone on payroll with their on-the-clock state,
// active first, alphabetically within.
func (s *Store) ListStaff(ctx context.Context) ([]Staff, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT st.id, st.name, st.email, st.phone, st.role, st.hourly_rate_cents, st.active, te.clock_in
		  FROM bm_staff st
		  LEFT JOIN bm_time_entries te ON te.staff_id = st.id AND te.clock_out IS NULL
		 ORDER BY st.active DESC, st.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Staff{}
	for rows.Next() {
		var st Staff
		if err := rows.Scan(&st.ID, &st.Name, &st.Email, &st.Phone, &st.Role,
			&st.HourlyRateCents, &st.Active, &st.OnClockSince); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// AddStaff inserts one staffer.
func (s *Store) AddStaff(ctx context.Context, st Staff) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_staff (name, email, phone, role, hourly_rate_cents)
		VALUES ($1, $2, $3, $4, $5)`,
		st.Name, st.Email, st.Phone, st.Role, st.HourlyRateCents)
	return err
}

// ClockToggle punches a staffer in or out: an open entry is closed, otherwise
// a new one opens. Returns true when the staffer is now on the clock.
func (s *Store) ClockToggle(ctx context.Context, staffID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE bm_time_entries SET clock_out = now()
		 WHERE staff_id = $1 AND clock_out IS NULL`, staffID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return false, nil // punched out
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO bm_time_entries (staff_id) VALUES ($1)`, staffID)
	return true, err
}

// WeekHours totals each active staffer's clocked time since weekStart (open
// entries count up to now).
func (s *Store) WeekHours(ctx context.Context, weekStart time.Time) ([]StaffHours, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT st.id, st.name,
		       COALESCE(SUM(EXTRACT(EPOCH FROM (COALESCE(te.clock_out, now()) - te.clock_in))) / 3600, 0),
		       bool_or(te.clock_out IS NULL AND te.clock_in IS NOT NULL)
		  FROM bm_staff st
		  LEFT JOIN bm_time_entries te ON te.staff_id = st.id AND te.clock_in >= $1
		 WHERE st.active
		 GROUP BY st.id, st.name
		 ORDER BY st.name`, weekStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaffHours{}
	for rows.Next() {
		var h StaffHours
		if err := rows.Scan(&h.StaffID, &h.Name, &h.Hours, &h.OnClock); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- shifts ---

// ListShifts returns shifts starting in [from, to), chronologically.
func (s *Store) ListShifts(ctx context.Context, from, to time.Time) ([]Shift, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sh.id, sh.staff_id, st.name, COALESCE(sh.property_id, 0), COALESCE(p.name, ''),
		       sh.starts_at, sh.ends_at, sh.kind, sh.notes
		  FROM bm_shifts sh
		  JOIN bm_staff st ON st.id = sh.staff_id
		  LEFT JOIN bm_properties p ON p.id = sh.property_id
		 WHERE sh.starts_at >= $1 AND sh.starts_at < $2
		 ORDER BY sh.starts_at, sh.id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Shift{}
	for rows.Next() {
		var sh Shift
		if err := rows.Scan(&sh.ID, &sh.StaffID, &sh.Staff, &sh.PropertyID, &sh.Property,
			&sh.StartsAt, &sh.EndsAt, &sh.Kind, &sh.Notes); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// AddShift inserts one scheduled block.
func (s *Store) AddShift(ctx context.Context, sh Shift) error {
	var prop any
	if sh.PropertyID != 0 {
		prop = sh.PropertyID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_shifts (staff_id, property_id, starts_at, ends_at, kind, notes)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		sh.StaffID, prop, sh.StartsAt, sh.EndsAt, sh.Kind, sh.Notes)
	return err
}
