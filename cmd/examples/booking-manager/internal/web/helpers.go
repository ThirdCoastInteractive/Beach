package web

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
	"github.com/a-h/templ"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

// View-data helpers shared across the .templ files: formatting, option
// shaping, and the status → badge-role vocabulary.

// deskRail returns the sidebar for signed-in users and nil for guests, which
// renders the stacked (rail-less) shell on the public pages.
func deskRail(a *app, active string, authed bool) templ.Component {
	if !authed {
		return nil
	}
	return a.sidebar(active)
}

// dollars renders cents as money, dropping trailing .00.
func dollars(cents int64) string {
	if cents%100 == 0 {
		return "$" + strconv.FormatInt(cents/100, 10)
	}
	return fmt.Sprintf("$%.2f", float64(cents)/100)
}

// qtyF renders a numeric quantity without a trailing .0.
func qtyF(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// firstWord is a person's short label on tight surfaces (calendar chips).
func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}

// dateShort is the tight in-app date form: "Jul 10".
func dateShort(t time.Time) string { return t.Format("Jan 2") }

// dateRange labels a stay: "Jul 10 – Jul 13".
func dateRange(in, out time.Time) string { return dateShort(in) + " – " + dateShort(out) }

// propertyMeta is a listing's one-line spec.
func propertyMeta(p store.Property) string {
	return fmt.Sprintf("Sleeps %d · %d br · %s/night", p.Sleeps, p.Bedrooms, dollars(p.NightlyRateCents))
}

// propertyOptions shapes properties into select options; withAny prepends the
// guest-facing "any property" choice.
func propertyOptions(props []store.Property, withAny bool) []driftwood.Option {
	opts := make([]driftwood.Option, 0, len(props)+1)
	if withAny {
		opts = append(opts, driftwood.Option{Value: "0", Label: "Any property"})
	}
	for _, p := range props {
		opts = append(opts, driftwood.Option{Value: strconv.FormatInt(p.ID, 10), Label: p.Name})
	}
	return opts
}

// Status → badge-role vocabulary. Roles are semantic accents, never colors.

func bookingRole(status string) driftwood.Role {
	switch status {
	case "pending":
		return driftwood.RoleWarn
	case "confirmed":
		return driftwood.RoleGood
	case "checked_in":
		return driftwood.RoleInfo
	case "cancelled":
		return driftwood.RoleDanger
	default: // checked_out
		return driftwood.RoleQuiet
	}
}

func inquiryRole(status string) driftwood.Role {
	switch status {
	case "new":
		return driftwood.RoleWarn
	case "quoted":
		return driftwood.RoleInfo
	case "won":
		return driftwood.RoleGood
	default: // lost
		return driftwood.RoleQuiet
	}
}

func stageRole(stage string) driftwood.Role {
	switch stage {
	case "interview":
		return driftwood.RoleInfo
	case "offer":
		return driftwood.RoleWarn
	case "hired":
		return driftwood.RoleGood
	case "rejected":
		return driftwood.RoleQuiet
	default: // applied
		return driftwood.RoleNeutral
	}
}

// statusLabel prints an enum value for humans: "checked_in" → "checked in".
func statusLabel(s string) string { return strings.ReplaceAll(s, "_", " ") }

// doorCode mints a random 4-digit guest code.
func doorCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		panic(err) // crypto/rand failing means the platform is broken
	}
	return fmt.Sprintf("%04d", n.Int64())
}
