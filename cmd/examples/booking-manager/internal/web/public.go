package web

import (
	"strconv"
	"strings"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// homePage is the public landing: the properties on offer and the inquiry
// form. Guests see this without an account; the operator's desk lives behind
// /dashboard.
func (a *app) homePage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	props, err := a.store.ListProperties(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.homeView(authed, props)}, nil
}

// createInquiry handles the public intake POST: validate, store, acknowledge
// the guest by email, and swap the form for a thank-you note.
func (a *app) createInquiry(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Name       string  `json:"name"`
		Email      string  `json:"email"`
		Phone      string  `json:"phone"`
		PartySize  float64 `json:"party_size"`
		PropertyID string  `json:"property_id"`
		CheckIn    string  `json:"check_in"`
		CheckOut   string  `json:"check_out"`
		Message    string  `json:"message"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, beach.Invalid("name", "Tell us who you are.")
	}
	if strings.TrimSpace(in.Email) == "" {
		return nil, beach.Invalid("email", "We need an email to answer you.")
	}

	propID, _ := strconv.ParseInt(in.PropertyID, 10, 64)
	q := store.Inquiry{
		PropertyID: propID,
		Name:       strings.TrimSpace(in.Name),
		Email:      strings.TrimSpace(in.Email),
		Phone:      strings.TrimSpace(in.Phone),
		PartySize:  int(in.PartySize),
		CheckIn:    parseDate(in.CheckIn),
		CheckOut:   parseDate(in.CheckOut),
		Message:    strings.TrimSpace(in.Message),
	}
	if q.PartySize < 1 {
		q.PartySize = 1
	}
	if err := a.store.AddInquiry(c.Context(), q); err != nil {
		return nil, err
	}

	propertyName := "any of our places"
	if propID != 0 {
		if p, err := a.store.GetProperty(c.Context(), propID); err == nil {
			propertyName = p.Name
		}
	}
	a.guests.InquiryReceived(q.Name, q.Email, propertyName)

	return beach.Patches{{Fragment: inquiryThanks(q.Name), Target: "inquire-card"}}, nil
}

// parseDate reads a date input's value ("" = not given).
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}
