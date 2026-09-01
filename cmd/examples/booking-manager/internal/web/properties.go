package web

import (
	"strings"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/locks"
	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// propertiesPage lists the operation's properties with the create form.
func (a *app) propertiesPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	props, err := a.store.ListProperties(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.propertiesView(authed, a.principalCan(c, "properties:write"), props)}, nil
}

// propertyPage is one property's detail: spec, standing key codes, the smart
// lock's live status, and the next stays.
func (a *app) propertyPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	p, err := a.store.GetProperty(c.Context(), pathID(c))
	if err != nil {
		return beach.View{}, beach.ErrNotFound
	}
	codes, err := a.store.ListKeyCodes(c.Context(), p.ID)
	if err != nil {
		return beach.View{}, err
	}
	// The next 90 days of stays, so the detail page answers "who's coming".
	today := time.Now()
	stays, err := a.store.ListBookings(c.Context(), p.ID, today, today.AddDate(0, 0, 90))
	if err != nil {
		return beach.View{}, err
	}
	// The lock's last-known state, when one is connected. A provider error
	// renders as offline rather than failing the page — lock trouble is
	// exactly when the operator needs the key codes.
	var lock *locks.Status
	if p.LockDeviceID != "" {
		st, err := a.locks.Status(c.Context(), p.LockDeviceID)
		if err != nil {
			st = locks.Status{DeviceID: p.LockDeviceID}
		}
		lock = &st
	}
	return beach.View{Page: a.propertyDetailView(authed, a.principalCan(c, "properties:write"), p, codes, stays, lock)}, nil
}

// createProperty handles POST /properties and re-renders the list.
func (a *app) createProperty(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Address     string  `json:"address"`
		Sleeps      float64 `json:"sleeps"`
		Bedrooms    float64 `json:"bedrooms"`
		Rate        float64 `json:"rate"`
		LockDevice  string  `json:"lock_device_id"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, beach.Invalid("name", "The property needs a name.")
	}
	p := store.Property{
		Name:             strings.TrimSpace(in.Name),
		Description:      strings.TrimSpace(in.Description),
		Address:          strings.TrimSpace(in.Address),
		Sleeps:           max(int(in.Sleeps), 1),
		Bedrooms:         max(int(in.Bedrooms), 1),
		NightlyRateCents: int64(in.Rate * 100),
		LockDeviceID:     strings.TrimSpace(in.LockDevice),
	}
	if err := a.store.AddProperty(c.Context(), p); err != nil {
		return nil, err
	}
	props, err := a.store.ListProperties(c.Context())
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: a.propertyList(props), Target: "prop-list"}}, nil
}

// addKeyCode handles POST /properties/{id}/codes and re-renders the code list.
func (a *app) addKeyCode(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		Label string `json:"label"`
		Code  string `json:"code"`
	}](c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Label) == "" || strings.TrimSpace(in.Code) == "" {
		return nil, beach.Invalid("label", "A code needs a label and digits.")
	}
	propID := pathID(c)
	if err := a.store.AddKeyCode(c.Context(), propID, strings.TrimSpace(in.Label), strings.TrimSpace(in.Code)); err != nil {
		return nil, err
	}
	return a.codeListPatch(c, propID)
}

// toggleKeyCode flips a standing code active/inactive.
func (a *app) toggleKeyCode(c *beach.Ctx) (beach.Patches, error) {
	propID, err := a.store.ToggleKeyCode(c.Context(), pathID(c))
	if err != nil {
		return nil, beach.ErrNotFound
	}
	return a.codeListPatch(c, propID)
}

func (a *app) codeListPatch(c *beach.Ctx, propID int64) (beach.Patches, error) {
	codes, err := a.store.ListKeyCodes(c.Context(), propID)
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: a.codeList(propID, codes), Target: "code-list"}}, nil
}
