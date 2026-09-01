package web

import (
	"strconv"
	"strings"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// inventoryPage lists supplies, optionally filtered to one property. The
// filter rides ?property= so it survives the adjust patches and the back
// button.
func (a *app) inventoryPage(c *beach.Ctx) (beach.View, error) {
	_, authed := c.User()
	propID, _ := strconv.ParseInt(c.Query("property"), 10, 64)
	supplies, err := a.store.ListSupplies(c.Context(), propID)
	if err != nil {
		return beach.View{}, err
	}
	props, err := a.store.ListProperties(c.Context())
	if err != nil {
		return beach.View{}, err
	}
	return beach.View{Page: a.inventoryView(authed, a.principalCan(c, "inventory:write"), propID, supplies, props)}, nil
}

// createSupply adds one stocked item and reloads onto its property's filter.
func (a *app) createSupply(c *beach.Ctx) (beach.Patches, error) {
	in, err := beach.Bind[struct {
		PropertyID string  `json:"sup_property_id"`
		Name       string  `json:"sup_name"`
		Category   string  `json:"category"`
		Qty        float64 `json:"qty"`
		Par        float64 `json:"par"`
		Unit       string  `json:"unit"`
	}](c)
	if err != nil {
		return nil, err
	}
	propID, _ := strconv.ParseInt(in.PropertyID, 10, 64)
	if propID == 0 {
		return nil, beach.Invalid("sup_property_id", "Pick the property this lives at.")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, beach.Invalid("sup_name", "The item needs a name.")
	}
	if err := a.store.AddSupply(c.Context(), store.Supply{
		PropertyID: propID,
		Name:       strings.TrimSpace(in.Name),
		Category:   firstNonEmpty(in.Category, "kitchen"),
		Qty:        in.Qty,
		Par:        in.Par,
		Unit:       firstNonEmpty(strings.TrimSpace(in.Unit), "ea"),
	}); err != nil {
		return nil, err
	}
	return beach.Patches{{Redirect: "/inventory?property=" + strconv.FormatInt(propID, 10)}}, nil
}

// adjustSupply nudges a count up or down and patches the list in place —
// counting is the control staff hammer during a turnover. The current filter
// rides the query string so the re-render stays on it.
func (a *app) adjustSupply(c *beach.Ctx) (beach.Patches, error) {
	delta, err := strconv.ParseFloat(c.Query("d"), 64)
	if err != nil || delta == 0 {
		return nil, beach.ErrBadRequest
	}
	if err := a.store.AdjustSupply(c.Context(), pathID(c), delta); err != nil {
		return nil, err
	}
	filter, _ := strconv.ParseInt(c.Query("property"), 10, 64)
	supplies, err := a.store.ListSupplies(c.Context(), filter)
	if err != nil {
		return nil, err
	}
	return beach.Patches{{Fragment: a.supplyList(filter, supplies, true), Target: "supply-list"}}, nil
}
