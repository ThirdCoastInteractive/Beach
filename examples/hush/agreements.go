// Legal assent. The terms page is static words; what matters operationally is
// (a) the checkboxes are enforced server-side — a request without one bounces,
// whatever the client claimed — and (b) every assent leaves a row. Two kinds:
// "tos-18" (the 18+/terms agreement at the three signup doors) and "2257"
// (the 18 U.S.C. § 2257 certification that rides every image upload).
package main

import (
	"context"
	"net/http"
)

const (
	agreeTOS18 = "tos-18"
	agree2257  = "2257"
)

// agreed reports whether the named checkbox arrived checked. Browsers send
// "on" for a bare checked box; anything present counts, absence doesn't.
func agreed(r *http.Request, field string) bool {
	return r.FormValue(field) != ""
}

// recordAgreement writes one assent row — best-effort, like the rest of the
// capture trail: a recording failure never blocks the action it witnesses,
// and database-less mode keeps no records (it keeps no anything).
func (a *app) recordAgreement(ctx context.Context, handle, kind string, m Meta) {
	if a.pool == nil {
		return
	}
	_, _ = a.pool.Exec(ctx,
		`INSERT INTO hush_agreements (handle, kind, ip, user_agent) VALUES ($1, $2, $3, $4)`,
		handle, kind, m.IP, m.UA)
}
