package store

import (
	"context"
	"time"
)

// Inquiry is one intake-form submission working the pipeline
// new -> quoted -> won|lost.
type Inquiry struct {
	ID         int64
	PropertyID int64 // 0 = "any property"
	Property   string
	Name       string
	Email      string
	Phone      string
	PartySize  int
	CheckIn    *time.Time
	CheckOut   *time.Time
	Message    string
	Status     string
	CreatedAt  time.Time
}

// Booking is a dated stay at one property. DoorCode is the per-stay code
// programmed on confirmation; LockDeviceID rides along from the property so
// the confirm handler can program the lock without a second read.
type Booking struct {
	ID           int64
	PropertyID   int64
	Property     string
	LockDeviceID string
	GuestName    string
	GuestEmail   string
	GuestPhone   string
	CheckIn      time.Time
	CheckOut     time.Time
	Status       string
	RateCents    int64
	DoorCode     string
	Notes        string
}

// --- inquiries ---

// ListInquiries returns inquiries, open pipeline first, newest first within a
// status.
func (s *Store) ListInquiries(ctx context.Context) ([]Inquiry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, COALESCE(i.property_id, 0), COALESCE(p.name, ''), i.name, i.email, i.phone,
		       i.party_size, i.check_in, i.check_out, i.message, i.status, i.created_at
		  FROM bm_inquiries i
		  LEFT JOIN bm_properties p ON p.id = i.property_id
		 ORDER BY (i.status IN ('won', 'lost')), i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Inquiry{}
	for rows.Next() {
		var q Inquiry
		if err := rows.Scan(&q.ID, &q.PropertyID, &q.Property, &q.Name, &q.Email, &q.Phone,
			&q.PartySize, &q.CheckIn, &q.CheckOut, &q.Message, &q.Status, &q.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetInquiry returns one inquiry by id.
func (s *Store) GetInquiry(ctx context.Context, id int64) (Inquiry, error) {
	var q Inquiry
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, COALESCE(i.property_id, 0), COALESCE(p.name, ''), i.name, i.email, i.phone,
		       i.party_size, i.check_in, i.check_out, i.message, i.status, i.created_at
		  FROM bm_inquiries i
		  LEFT JOIN bm_properties p ON p.id = i.property_id
		 WHERE i.id = $1`, id).
		Scan(&q.ID, &q.PropertyID, &q.Property, &q.Name, &q.Email, &q.Phone,
			&q.PartySize, &q.CheckIn, &q.CheckOut, &q.Message, &q.Status, &q.CreatedAt)
	return q, err
}

// AddInquiry inserts one intake submission.
func (s *Store) AddInquiry(ctx context.Context, q Inquiry) error {
	var prop any
	if q.PropertyID != 0 {
		prop = q.PropertyID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_inquiries (property_id, name, email, phone, party_size, check_in, check_out, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		prop, q.Name, q.Email, q.Phone, q.PartySize, dateOrNil(q.CheckIn), dateOrNil(q.CheckOut), q.Message)
	return err
}

// Date columns are always sent as ISO date strings, never time.Time: a
// timestamptz parameter compared against a date column converts through the
// server's time zone and can land a day off.

func isoDate(t time.Time) string { return t.Format("2006-01-02") }

func dateOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return isoDate(*t)
}

// SetInquiryStatus moves an inquiry through the pipeline.
func (s *Store) SetInquiryStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE bm_inquiries SET status = $2 WHERE id = $1`, id, status)
	return err
}

// --- bookings ---

const bookingCols = `
	b.id, b.property_id, p.name, COALESCE(p.lock_device_id, ''), b.guest_name, b.guest_email, b.guest_phone,
	b.check_in, b.check_out, b.status, b.rate_cents, b.door_code, b.notes`

func scanBooking(row interface{ Scan(...any) error }, b *Booking) error {
	return row.Scan(&b.ID, &b.PropertyID, &b.Property, &b.LockDeviceID, &b.GuestName,
		&b.GuestEmail, &b.GuestPhone, &b.CheckIn, &b.CheckOut, &b.Status,
		&b.RateCents, &b.DoorCode, &b.Notes)
}

// ListBookings returns bookings overlapping [from, to), optionally filtered
// to one property (0 = all), earliest check-in first. Cancelled stays are
// included — the list view badges them — but conflict checks ignore them.
func (s *Store) ListBookings(ctx context.Context, propertyID int64, from, to time.Time) ([]Booking, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+bookingCols+`
		  FROM bm_bookings b
		  JOIN bm_properties p ON p.id = b.property_id
		 WHERE b.check_in < $2::date AND b.check_out > $1::date
		   AND ($3 = 0 OR b.property_id = $3)
		 ORDER BY b.check_in, b.id`, isoDate(from), isoDate(to), propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Booking{}
	for rows.Next() {
		var b Booking
		if err := scanBooking(rows, &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBooking returns one booking by id, property name and lock device
// attached.
func (s *Store) GetBooking(ctx context.Context, id int64) (Booking, error) {
	var b Booking
	err := scanBooking(s.pool.QueryRow(ctx, `
		SELECT `+bookingCols+`
		  FROM bm_bookings b
		  JOIN bm_properties p ON p.id = b.property_id
		 WHERE b.id = $1`, id), &b)
	return b, err
}

// AddBooking inserts one stay and returns its id. InquiryID links the intake
// it came from (0 = none).
func (s *Store) AddBooking(ctx context.Context, b Booking, inquiryID int64) (int64, error) {
	var inq any
	if inquiryID != 0 {
		inq = inquiryID
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO bm_bookings (property_id, inquiry_id, guest_name, guest_email, guest_phone,
		                         check_in, check_out, status, rate_cents, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		b.PropertyID, inq, b.GuestName, b.GuestEmail, b.GuestPhone,
		isoDate(b.CheckIn), isoDate(b.CheckOut), b.Status, b.RateCents, b.Notes).Scan(&id)
	return id, err
}

// SetBookingStatus moves a booking through its lifecycle.
func (s *Store) SetBookingStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE bm_bookings SET status = $2 WHERE id = $1`, id, status)
	return err
}

// SetBookingDoorCode stores the per-stay code the locks provider programmed.
func (s *Store) SetBookingDoorCode(ctx context.Context, id int64, code string) error {
	_, err := s.pool.Exec(ctx, `UPDATE bm_bookings SET door_code = $2 WHERE id = $1`, id, code)
	return err
}

// HasConflict reports whether a live (non-cancelled) booking already occupies
// the property for any night of [checkIn, checkOut). exclude skips one
// booking id (0 = none) so re-confirming a stay doesn't collide with itself.
func (s *Store) HasConflict(ctx context.Context, propertyID int64, checkIn, checkOut time.Time, exclude int64) (bool, error) {
	var conflict bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM bm_bookings
			 WHERE property_id = $1
			   AND status <> 'cancelled'
			   AND id <> $4
			   AND check_in < $3::date AND check_out > $2::date
		)`, propertyID, isoDate(checkIn), isoDate(checkOut), exclude).Scan(&conflict)
	return conflict, err
}

// BookingsArriving returns confirmed stays checking in on day.
func (s *Store) BookingsArriving(ctx context.Context, day time.Time) ([]Booking, error) {
	return s.bookingsOn(ctx, `b.check_in = $1::date AND b.status IN ('pending', 'confirmed')`, day)
}

// BookingsDeparting returns stays checking out on day.
func (s *Store) BookingsDeparting(ctx context.Context, day time.Time) ([]Booking, error) {
	return s.bookingsOn(ctx, `b.check_out = $1::date AND b.status IN ('confirmed', 'checked_in')`, day)
}

func (s *Store) bookingsOn(ctx context.Context, where string, day time.Time) ([]Booking, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+bookingCols+`
		  FROM bm_bookings b
		  JOIN bm_properties p ON p.id = b.property_id
		 WHERE `+where+`
		 ORDER BY p.name`, isoDate(day))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Booking{}
	for rows.Next() {
		var b Booking
		if err := scanBooking(rows, &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CountInquiries returns how many inquiries sit at a status.
func (s *Store) CountInquiries(ctx context.Context, status string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM bm_inquiries WHERE status = $1`, status).Scan(&n)
	return n, err
}
