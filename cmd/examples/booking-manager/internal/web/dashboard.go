package web

import (
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// dashboardData is the day sheet: what today demands, shaped in Go so the
// template stays markup.
type dashboardData struct {
	NewInquiries int
	Arriving     []store.Booking
	Departing    []store.Booking
	Shifts       []store.Shift
	Low          []store.Supply
}

// dashboardPage is the desk's home: today's arrivals and departures, the
// unanswered inquiries, today's shifts, and what needs restocking.
func (a *app) dashboardPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	ctx := c.Context()
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	var d dashboardData
	var err error
	if d.NewInquiries, err = a.store.CountInquiries(ctx, "new"); err != nil {
		return beach.View{}, err
	}
	if d.Arriving, err = a.store.BookingsArriving(ctx, now); err != nil {
		return beach.View{}, err
	}
	if d.Departing, err = a.store.BookingsDeparting(ctx, now); err != nil {
		return beach.View{}, err
	}
	if d.Shifts, err = a.store.ListShifts(ctx, dayStart, dayStart.AddDate(0, 0, 1)); err != nil {
		return beach.View{}, err
	}
	if d.Low, err = a.store.LowSupplies(ctx); err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.dashboardView(authed, d)}, nil
}

// shiftMeta labels one shift line.
func shiftMeta(sh store.Shift) string {
	meta := sh.StartsAt.Format("3:04pm") + " – " + sh.EndsAt.Format("3:04pm") + " · " + sh.Kind
	if sh.Property != "" {
		meta += " · " + sh.Property
	}
	return meta
}

// arrivalMeta labels an arrival/departure line.
func arrivalMeta(b store.Booking) string {
	meta := b.Property + " · " + dateRange(b.CheckIn, b.CheckOut)
	if b.DoorCode != "" {
		meta += " · door code " + b.DoorCode
	}
	return meta
}
