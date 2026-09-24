package web

import (
	"fmt"
	"strconv"

	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// Row/desc shapers called from the .templ files. They keep the templates
// reading as plain markup.

// propertyRows shapes the property list; each row links to its detail page.
func propertyRows(props []store.Property) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(props))
	for _, p := range props {
		rows = append(rows, driftwood.ListRow{
			Title: p.Name,
			Meta:  propertyMeta(p) + " · " + p.Address,
			Href:  "/properties/" + strconv.FormatInt(p.ID, 10),
		})
	}
	return rows
}

// propertyDetails is the detail page's spec sheet.
func propertyDetails(p store.Property) []driftwood.DescItem {
	items := []driftwood.DescItem{
		{Term: "Address", Value: p.Address},
		{Term: "Sleeps", Value: strconv.Itoa(p.Sleeps)},
		{Term: "Bedrooms", Value: strconv.Itoa(p.Bedrooms)},
		{Term: "Nightly rate", Value: dollars(p.NightlyRateCents)},
	}
	if p.LockDeviceID != "" {
		items = append(items, driftwood.DescItem{Term: "Lock device", Value: p.LockDeviceID})
	}
	return items
}

// stayRows shapes a property's upcoming bookings.
func stayRows(stays []store.Booking) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(stays))
	for _, b := range stays {
		rows = append(rows, driftwood.ListRow{
			Title: b.GuestName,
			Meta:  dateRange(b.CheckIn, b.CheckOut),
			Badge: &driftwood.BadgeProps{Label: statusLabel(b.Status), Role: bookingRole(b.Status)},
		})
	}
	return rows
}

// codeRows shapes a property's standing key codes, toggle attached.
func codeRows(codes []store.KeyCode) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(codes))
	for _, k := range codes {
		badge := &driftwood.BadgeProps{Label: "active", Role: driftwood.RoleGood, Dot: true}
		if !k.Active {
			badge = &driftwood.BadgeProps{Label: "disabled", Role: driftwood.RoleQuiet}
		}
		rows = append(rows, driftwood.ListRow{
			Title:  k.Label + " · " + k.Code,
			Meta:   "added " + dateShort(k.CreatedAt),
			Badge:  badge,
			Action: codeToggle(k),
		})
	}
	return rows
}

// toggleLabel names the key-code toggle by what it will do.
func toggleLabel(k store.KeyCode) string {
	if k.Active {
		return "Disable"
	}
	return "Enable"
}

// daySheetRows shapes the dashboard's arrival/departure lines.
func daySheetRows(bookings []store.Booking) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(bookings))
	for _, b := range bookings {
		rows = append(rows, driftwood.ListRow{
			Title: b.GuestName,
			Meta:  arrivalMeta(b),
			Badge: &driftwood.BadgeProps{Label: statusLabel(b.Status), Role: bookingRole(b.Status)},
		})
	}
	return rows
}

// shiftSheetRows shapes the dashboard's shift lines.
func shiftSheetRows(shifts []store.Shift) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(shifts))
	for _, sh := range shifts {
		rows = append(rows, driftwood.ListRow{Title: sh.Staff, Meta: shiftMeta(sh)})
	}
	return rows
}

// lowSupplyRows shapes the restock list.
func lowSupplyRows(low []store.Supply) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(low))
	for _, sp := range low {
		rows = append(rows, driftwood.ListRow{
			Title: sp.Name,
			Meta:  sp.Property + " · " + qtyF(sp.Qty) + " " + sp.Unit + " on hand, par " + qtyF(sp.Par),
			Badge: &driftwood.BadgeProps{Label: "low", Role: driftwood.RoleWarn, Dot: true},
		})
	}
	return rows
}

// supplyPost builds the @post expression for a count adjustment, carrying
// the current property filter so the patch re-renders the same view.
func supplyPost(filter, id int64, delta int) string {
	return "@post('/supplies/" + strconv.FormatInt(id, 10) + "/adjust?d=" + strconv.Itoa(delta) +
		"&property=" + strconv.FormatInt(filter, 10) + "')"
}

// supplyRows shapes the counts, low-stock badged, adjusters attached for
// writers.
func supplyRows(filter int64, supplies []store.Supply, canWrite bool) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(supplies))
	for _, sp := range supplies {
		row := driftwood.ListRow{
			Title: sp.Name,
			Meta:  sp.Property + " · " + sp.Category + " · " + qtyF(sp.Qty) + " " + sp.Unit + " on hand, par " + qtyF(sp.Par),
		}
		if sp.Low() {
			row.Badge = &driftwood.BadgeProps{Label: "low", Role: driftwood.RoleWarn, Dot: true}
		}
		if canWrite {
			row.Action = supplyAdjust(filter, sp)
		}
		rows = append(rows, row)
	}
	return rows
}

// categoryOptions is the supply-category vocabulary.
func categoryOptions() []driftwood.Option {
	return []driftwood.Option{
		{Value: "kitchen", Label: "Kitchen"},
		{Value: "cleaning", Label: "Cleaning"},
		{Value: "linens", Label: "Linens"},
		{Value: "other", Label: "Other"},
	}
}

// applicantPost builds the @post expression for a hiring-pipeline action.
func applicantPost(id int64, tail string) string {
	return "@post('/applicants/" + strconv.FormatInt(id, 10) + "/" + tail + "')"
}

// applicantRows shapes the hiring pipeline, actions attached for writers.
func applicantRows(apps []store.Applicant, canWrite bool) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(apps))
	for _, ap := range apps {
		meta := ap.Role + " · " + ap.Email
		if ap.Phone != "" {
			meta += " · " + ap.Phone
		}
		if ap.Notes != "" {
			meta += " · " + ap.Notes
		}
		row := driftwood.ListRow{
			Title: ap.Name,
			Meta:  meta,
			Badge: &driftwood.BadgeProps{Label: ap.Stage, Role: stageRole(ap.Stage)},
		}
		if canWrite {
			row.Action = applicantActions(ap)
		}
		rows = append(rows, row)
	}
	return rows
}

// staffRows shapes payroll; the clock button rides every active row.
func staffRows(staff []store.Staff) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(staff))
	for _, st := range staff {
		meta := st.Role
		if st.HourlyRateCents > 0 {
			meta += " · " + dollars(st.HourlyRateCents) + "/h"
		}
		if st.Email != "" {
			meta += " · " + st.Email
		}
		badge := &driftwood.BadgeProps{Label: "off the clock", Role: driftwood.RoleQuiet}
		if st.OnClockSince != nil {
			badge = &driftwood.BadgeProps{Label: "on since " + st.OnClockSince.Local().Format("3:04pm"), Role: driftwood.RoleGood, Dot: true}
		}
		row := driftwood.ListRow{Title: st.Name, Meta: meta, Badge: badge}
		if st.Active {
			row.Action = clockButton(st)
		}
		rows = append(rows, row)
	}
	return rows
}

// hourRows shapes the weekly hours table.
func hourRows(hours []store.StaffHours) [][]string {
	rows := make([][]string, 0, len(hours))
	for _, h := range hours {
		status := ""
		if h.OnClock {
			status = "on the clock"
		}
		rows = append(rows, []string{h.Name, fmt.Sprintf("%.1f h", h.Hours), status})
	}
	return rows
}

// shiftRows shapes the week's schedule.
func shiftRows(shifts []store.Shift) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(shifts))
	for _, sh := range shifts {
		meta := sh.StartsAt.Format("Mon Jan 2") + " · " + shiftMeta(sh)
		if sh.Notes != "" {
			meta += " · " + sh.Notes
		}
		rows = append(rows, driftwood.ListRow{Title: sh.Staff, Meta: meta})
	}
	return rows
}

// staffOptions shapes payroll into select options.
func staffOptions(staff []store.Staff) []driftwood.Option {
	opts := make([]driftwood.Option, 0, len(staff))
	for _, st := range staff {
		if !st.Active {
			continue
		}
		opts = append(opts, driftwood.Option{Value: strconv.FormatInt(st.ID, 10), Label: st.Name})
	}
	return opts
}

// shiftPropertyOptions is propertyOptions with the "general work" zero row.
func shiftPropertyOptions(props []store.Property) []driftwood.Option {
	opts := []driftwood.Option{{Value: "0", Label: "General"}}
	return append(opts, propertyOptions(props, false)...)
}

// roleOptions is the staff-role vocabulary.
func roleOptions() []driftwood.Option {
	return []driftwood.Option{
		{Value: "cleaner", Label: "Cleaner"},
		{Value: "maintenance", Label: "Maintenance"},
		{Value: "manager", Label: "Manager"},
	}
}

// kindOptions is the shift-kind vocabulary.
func kindOptions() []driftwood.Option {
	return []driftwood.Option{
		{Value: "cleaning", Label: "Cleaning"},
		{Value: "turnover", Label: "Turnover"},
		{Value: "maintenance", Label: "Maintenance"},
		{Value: "greeting", Label: "Guest greeting"},
	}
}

// inquiryPost builds the @post expression for an inquiry action.
func inquiryPost(id int64, tail string) string {
	return "@post('/inquiries/" + strconv.FormatInt(id, 10) + "/" + tail + "')"
}

// bookingPost builds the @post expression for a booking status change.
func bookingPost(id int64, to string) string {
	return "@post('/bookings/" + strconv.FormatInt(id, 10) + "/status?to=" + to + "')"
}

// inquiryRows shapes the intake pipeline, actions attached for writers.
func inquiryRows(inqs []store.Inquiry, canWrite bool) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(inqs))
	for _, q := range inqs {
		title := q.Name + " — " + firstNonEmpty(q.Property, "any property")
		meta := q.Email
		if q.Phone != "" {
			meta += " · " + q.Phone
		}
		if q.CheckIn != nil && q.CheckOut != nil {
			meta = dateRange(*q.CheckIn, *q.CheckOut) + " · " + strconv.Itoa(q.PartySize) + " guests · " + meta
		}
		if q.Message != "" {
			meta += " · “" + q.Message + "”"
		}
		row := driftwood.ListRow{
			Title: title,
			Meta:  meta,
			Badge: &driftwood.BadgeProps{Label: q.Status, Role: inquiryRole(q.Status)},
		}
		if canWrite {
			row.Action = inquiryActions(q)
		}
		rows = append(rows, row)
	}
	return rows
}

// bookingRows shapes a month's stays, lifecycle actions attached for writers.
func bookingRows(bookings []store.Booking, canWrite bool) []driftwood.ListRow {
	rows := make([]driftwood.ListRow, 0, len(bookings))
	for _, b := range bookings {
		meta := dateRange(b.CheckIn, b.CheckOut) + " · " + dollars(b.RateCents) + "/night"
		if b.GuestEmail != "" {
			meta += " · " + b.GuestEmail
		}
		if b.DoorCode != "" {
			meta += " · door code " + b.DoorCode
		}
		row := driftwood.ListRow{
			Title: b.GuestName + " — " + b.Property,
			Meta:  meta,
			Badge: &driftwood.BadgeProps{Label: statusLabel(b.Status), Role: bookingRole(b.Status)},
		}
		if canWrite {
			row.Action = bookingActions(b)
		}
		rows = append(rows, row)
	}
	return rows
}
