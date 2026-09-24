package web

import (
	"strconv"
	"strings"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// --- hiring ---

// hiringPage lists the applicant pipeline with the add form.
func (a *app) hiringPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	apps, err := a.store.ListApplicants(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.hiringView(authed, a.principalCan(c, "staffing:write"), apps)}, nil
}

// createApplicant adds a candidate at the applied stage.
func (a *app) createApplicant(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
		Role  string `json:"role"`
		Notes string `json:"notes"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, beach.Invalid("name", "The applicant needs a name.")
	}
	if err := a.store.AddApplicant(c.Context(), store.Applicant{
		Name:  strings.TrimSpace(in.Name),
		Email: strings.TrimSpace(in.Email),
		Phone: strings.TrimSpace(in.Phone),
		Role:  firstNonEmpty(in.Role, "cleaner"),
		Notes: strings.TrimSpace(in.Notes),
	}); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/hiring"}}, nil
}

// setApplicantStage advances (or ends) a candidate's pipeline.
func (a *app) setApplicantStage(c *beach.Ctx) (beach.Patches, error) {
	to := c.Query("to")
	switch to {
	case "interview", "offer", "rejected":
	default:
		return nil, beach.ErrBadRequest
	}
	if _, err := a.store.SetApplicantStage(c.Context(), pathID(c), to); err != nil {
		return nil, beach.ErrNotFound
	}
	return beach.Patches{{Redirect: "/hiring"}}, nil
}

// hireApplicant closes the pipeline for a candidate and puts them on staff.
func (a *app) hireApplicant(c *beach.Ctx) (beach.Patches, error) {
	app, err := a.store.SetApplicantStage(c.Context(), pathID(c), "hired")
	if err != nil {
		return nil, beach.ErrNotFound
	}
	if err := a.store.AddStaff(c.Context(), store.Staff{
		Name:  app.Name,
		Email: app.Email,
		Phone: app.Phone,
		Role:  app.Role,
	}); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/staff"}}, nil
}

// --- staff, shifts, and the clock ---

// staffPage is the staffing desk: who's on payroll (and on the clock), this
// week's schedule, the week's clocked hours, and the add forms.
func (a *app) staffPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	ctx := c.Context()

	staff, err := a.store.ListStaff(ctx)
	if err != nil {
		return beach.View{}, err
	}
	weekStart := startOfWeek(time.Now())
	shifts, err := a.store.ListShifts(ctx, weekStart, weekStart.AddDate(0, 0, 7))
	if err != nil {
		return beach.View{}, err
	}
	hours, err := a.store.WeekHours(ctx, weekStart)
	if err != nil {
		return beach.View{}, err
	}
	props, err := a.store.ListProperties(ctx)
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.staffView(authed, a.principalCan(c, "staffing:write"), staff, shifts, hours, props, weekStart)}, nil
}

// createStaff adds someone straight to payroll (hired outside the pipeline).
func (a *app) createStaff(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Name  string  `json:"name"`
		Email string  `json:"email"`
		Phone string  `json:"phone"`
		Role  string  `json:"role"`
		Rate  float64 `json:"rate"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, beach.Invalid("name", "The staffer needs a name.")
	}
	if err := a.store.AddStaff(c.Context(), store.Staff{
		Name:            strings.TrimSpace(in.Name),
		Email:           strings.TrimSpace(in.Email),
		Phone:           strings.TrimSpace(in.Phone),
		Role:            firstNonEmpty(in.Role, "cleaner"),
		HourlyRateCents: int64(in.Rate * 100),
	}); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/staff"}}, nil
}

// createShift schedules one block of work.
func (a *app) createShift(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		StaffID    string  `json:"staff_id"`
		PropertyID string  `json:"shift_property_id"`
		Day        string  `json:"day"`
		Start      string  `json:"start"`
		Hours      float64 `json:"hours"`
		Kind       string  `json:"kind"`
		Notes      string  `json:"shift_notes"`
	}](c)
	if err != nil {
		return nil, err
	}
	staffID, _ := strconv.ParseInt(in.StaffID, 10, 64)
	if staffID == 0 {
		return nil, beach.Invalid("staff_id", "Pick who works the shift.")
	}
	day := parseDate(in.Day)
	if day == nil {
		return nil, beach.Invalid("day", "Pick the day.")
	}
	start, err := time.Parse("15:04", firstNonEmpty(in.Start, "09:00"))
	if err != nil {
		return nil, beach.Invalid("start", "Start time looks off.")
	}
	if in.Hours <= 0 {
		in.Hours = 4
	}
	propID, _ := strconv.ParseInt(in.PropertyID, 10, 64)
	startsAt := time.Date(day.Year(), day.Month(), day.Day(), start.Hour(), start.Minute(), 0, 0, time.Local)
	if err := a.store.AddShift(c.Context(), store.Shift{
		StaffID:    staffID,
		PropertyID: propID,
		StartsAt:   startsAt,
		EndsAt:     startsAt.Add(time.Duration(in.Hours * float64(time.Hour))),
		Kind:       firstNonEmpty(in.Kind, "cleaning"),
		Notes:      strings.TrimSpace(in.Notes),
	}); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/staff"}}, nil
}

// clockToggle punches a staffer in or out and re-renders the staff list in
// place — the clock is the one control staff hammer all day, so it patches
// instead of reloading.
func (a *app) clockToggle(c *beach.Ctx) (beach.Patches, error) {
	if _, err := a.store.ClockToggle(c.Context(), pathID(c)); err != nil {
		return nil, beach.ErrNotFound
	}
	staff, err := a.store.ListStaff(c.Context())
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: staffList(staff), Target: "staff-list"}}, nil
}

// startOfWeek returns the most recent Sunday at midnight local.
func startOfWeek(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return day.AddDate(0, 0, -int(day.Weekday()))
}
