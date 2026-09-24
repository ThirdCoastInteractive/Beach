package web

import (
	"strconv"
	"strings"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// --- inquiries ---

// inquiriesPage lists the intake pipeline.
func (a *app) inquiriesPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	inqs, err := a.store.ListInquiries(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.inquiriesView(authed, a.principalCan(c, "bookings:write"), inqs)}, nil
}

// setInquiryStatus moves an inquiry along the pipeline (quote it, mark it
// lost) and reloads the page — status changes are real navigations.
func (a *app) setInquiryStatus(c *beach.Ctx) (beach.Patches, error) {
	to := c.Query("to")
	switch to {
	case "quoted", "lost", "new":
	default:
		return nil, beach.ErrBadRequest
	}
	if err := a.store.SetInquiryStatus(c.Context(), pathID(c), to); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/inquiries"}}, nil
}

// bookInquiry converts a dated inquiry into a pending booking at the
// property's nightly rate and marks the inquiry won.
func (a *app) bookInquiry(c *beach.Ctx) (beach.Patches, error) {
	q, err := a.store.GetInquiry(c.Context(), pathID(c))
	if err != nil {
		return nil, beach.ErrNotFound
	}
	if q.PropertyID == 0 || q.CheckIn == nil || q.CheckOut == nil {
		// The convert button only renders on complete inquiries, so this is a
		// stale click; say why instead of guessing dates.
		return a.inquiryNote("This inquiry needs a property and dates before it can become a booking."), nil
	}
	conflict, err := a.store.HasConflict(c.Context(), q.PropertyID, *q.CheckIn, *q.CheckOut, 0)
	if err != nil {
		return nil, err
	}
	if conflict {
		return a.inquiryNote("Those dates collide with a stay already on the books for " + q.Property + "."), nil
	}
	p, err := a.store.GetProperty(c.Context(), q.PropertyID)
	if err != nil {
		return nil, err
	}
	if _, err := a.store.AddBooking(c.Context(), store.Booking{
		PropertyID: q.PropertyID,
		GuestName:  q.Name,
		GuestEmail: q.Email,
		GuestPhone: q.Phone,
		CheckIn:    *q.CheckIn,
		CheckOut:   *q.CheckOut,
		Status:     "pending",
		RateCents:  p.NightlyRateCents,
		Notes:      q.Message,
	}, q.ID); err != nil {
		return nil, err
	}
	if err := a.store.SetInquiryStatus(c.Context(), q.ID, "won"); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/bookings"}}, nil
}

// inquiryNote patches an explanation into the inquiries page's note slot.
func (a *app) inquiryNote(msg string) beach.Patches {
	return beach.Patches{{Fragment: noteAlert(msg), Target: "inq-note", Mode: beach.PatchInner}}
}

// --- bookings ---

// bookingsPage is the month calendar plus the month's stays and the manual
// booking form. Month and property filter ride the query string — tabs and
// filters are query params, never routes.
func (a *app) bookingsPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	month := parseMonth(c.Query("month"))
	propID, _ := strconv.ParseInt(c.Query("property"), 10, 64)

	props, err := a.store.ListProperties(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	next := month.AddDate(0, 1, 0)
	bookings, err := a.store.ListBookings(c.Context(), propID, month, next)
	if err != nil {
		return beach.View{}, err
	}
	cal := calendarData{
		Month:      month,
		Title:      month.Format("January 2006"),
		PrevHref:   monthHref(month.AddDate(0, -1, 0), propID),
		NextHref:   monthHref(next, propID),
		PropertyID: propID,
		Weeks:      buildCalendar(month, bookings),
	}
	return beach.View{Page: a.bookingsView(authed, a.principalCan(c, "bookings:write"), cal, bookings, props)}, nil
}

// createBooking handles the manual booking form (phone bookings, repeat
// guests). Conflicts answer with a note, not a write.
func (a *app) createBooking(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		PropertyID string  `json:"property_id"`
		GuestName  string  `json:"guest_name"`
		GuestEmail string  `json:"guest_email"`
		GuestPhone string  `json:"guest_phone"`
		CheckIn    string  `json:"check_in"`
		CheckOut   string  `json:"check_out"`
		Rate       float64 `json:"rate"`
	}](c)
	if err != nil {
		return nil, err
	}
	propID, _ := strconv.ParseInt(in.PropertyID, 10, 64)
	if propID == 0 {
		return nil, beach.Invalid("property_id", "Pick the property.")
	}
	if strings.TrimSpace(in.GuestName) == "" {
		return nil, beach.Invalid("guest_name", "Who is staying?")
	}
	checkIn, checkOut := parseDate(in.CheckIn), parseDate(in.CheckOut)
	if checkIn == nil || checkOut == nil || !checkOut.After(*checkIn) {
		return nil, beach.Invalid("check_in", "Check-out must come after check-in.")
	}
	conflict, err := a.store.HasConflict(c.Context(), propID, *checkIn, *checkOut, 0)
	if err != nil {
		return nil, err
	}
	if conflict {
		return beach.Patches{{Fragment: noteAlert("Those dates collide with a stay already on the books."), Target: "book-note", Mode: beach.PatchInner}}, nil
	}
	rate := int64(in.Rate * 100)
	if rate == 0 {
		if p, err := a.store.GetProperty(c.Context(), propID); err == nil {
			rate = p.NightlyRateCents
		}
	}
	if _, err := a.store.AddBooking(c.Context(), store.Booking{
		PropertyID: propID,
		GuestName:  strings.TrimSpace(in.GuestName),
		GuestEmail: strings.TrimSpace(in.GuestEmail),
		GuestPhone: strings.TrimSpace(in.GuestPhone),
		CheckIn:    *checkIn,
		CheckOut:   *checkOut,
		Status:     "pending",
		RateCents:  rate,
	}, 0); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: monthHref(time.Date(checkIn.Year(), checkIn.Month(), 1, 0, 0, 0, 0, time.Local), 0)}}, nil
}

// setBookingStatus moves a stay through its lifecycle. Confirmation is the
// big one: it programs a per-stay door code through the locks provider and
// notifies the guest by email and text; cancellation clears the code again.
func (a *app) setBookingStatus(c *beach.Ctx) (beach.Patches, error) {
	to := c.Query("to")
	switch to {
	case "confirmed", "checked_in", "checked_out", "cancelled":
	default:
		return nil, beach.ErrBadRequest
	}
	b, err := a.store.GetBooking(c.Context(), pathID(c))
	if err != nil {
		return nil, beach.ErrNotFound
	}

	if to == "confirmed" {
		conflict, err := a.store.HasConflict(c.Context(), b.PropertyID, b.CheckIn, b.CheckOut, b.ID)
		if err != nil {
			return nil, err
		}
		if conflict {
			return beach.Patches{{Fragment: noteAlert("Can't confirm: those dates collide with another stay."), Target: "book-note", Mode: beach.PatchInner}}, nil
		}
	}
	if err := a.store.SetBookingStatus(c.Context(), b.ID, to); err != nil {
		return nil, err
	}

	switch to {
	case "confirmed":
		code := b.DoorCode
		if b.LockDeviceID != "" {
			if code == "" {
				code = doorCode()
			}
			if err := a.locks.SetCode(c.Context(), b.LockDeviceID, guestSlot(b.ID), code); err != nil {
				// The stay stays confirmed; the operator sees the code failed to
				// program and can hand out a standing code instead.
				return beach.Patches{{Fragment: noteAlert("Confirmed, but programming the door code failed: " + err.Error()), Target: "book-note", Mode: beach.PatchInner}}, nil
			}
			if err := a.store.SetBookingDoorCode(c.Context(), b.ID, code); err != nil {
				return nil, err
			}
		}
		a.guests.BookingConfirmed(b.GuestName, b.GuestEmail, b.GuestPhone, b.Property, b.CheckIn, b.CheckOut, code)
	case "cancelled":
		if b.LockDeviceID != "" && b.DoorCode != "" {
			if err := a.locks.ClearCode(c.Context(), b.LockDeviceID, guestSlot(b.ID)); err == nil {
				_ = a.store.SetBookingDoorCode(c.Context(), b.ID, "")
			}
		}
	}
	return beach.Patches{{Redirect: "/bookings"}}, nil
}

// guestSlot names a booking's code slot on the lock.
func guestSlot(bookingID int64) string { return "guest-" + strconv.FormatInt(bookingID, 10) }

// --- calendar shaping ---

// calendarData is everything the month grid needs, shaped in Go so the
// template stays plain markup.
type calendarData struct {
	Month      time.Time
	Title      string
	PrevHref   string
	NextHref   string
	PropertyID int64
	Weeks      [][]calDay
}

type calDay struct {
	Day     int
	InMonth bool
	Today   bool
	Chips   []calChip
}

type calChip struct {
	Label string
	Role  driftwood.Role
}

// parseMonth reads ?month=2006-01, defaulting to the current month.
func parseMonth(s string) time.Time {
	if t, err := time.ParseInLocation("2006-01", s, time.Local); err == nil {
		return t
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
}

func monthHref(month time.Time, propID int64) string {
	href := "/bookings?month=" + month.Format("2006-01")
	if propID != 0 {
		href += "&property=" + strconv.FormatInt(propID, 10)
	}
	return href
}

// buildCalendar lays a month's bookings onto a Sunday-first grid. A stay
// chips every night it occupies (check-in inclusive, check-out exclusive);
// cancelled stays stay off the calendar.
func buildCalendar(month time.Time, bookings []store.Booking) [][]calDay {
	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.Local)
	start := first.AddDate(0, 0, -int(first.Weekday()))
	todayKey := time.Now().Format("2006-01-02")

	var weeks [][]calDay
	day := start
	for {
		week := make([]calDay, 7)
		for i := range week {
			key := day.Format("2006-01-02")
			d := calDay{Day: day.Day(), InMonth: day.Month() == month.Month(), Today: key == todayKey}
			for _, b := range bookings {
				if b.Status == "cancelled" {
					continue
				}
				if key >= b.CheckIn.Format("2006-01-02") && key < b.CheckOut.Format("2006-01-02") {
					d.Chips = append(d.Chips, calChip{Label: firstWord(b.GuestName), Role: bookingRole(b.Status)})
				}
			}
			week[i] = d
			day = day.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
		if day.Month() != month.Month() {
			break
		}
	}
	return weeks
}
