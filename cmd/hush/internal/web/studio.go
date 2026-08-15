package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	authx "github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/auth"
	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/content"
	"github.com/ThirdCoastInteractive/the-beach/cmd/hush/internal/store"

	beach "github.com/ThirdCoastInteractive/the-beach/pkg/beach"
)

// studio serves the BYOL canvas.
func (a *App) studio(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: studioPage(studioNotice(c.Query("err")))}, nil
}

// uploadPiece stores one studio image and returns the ids the client needs
// to keep arranging. It does not create a post — publishLayout does that.
func (a *App) uploadPiece(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !authx.Agreed(r, "cert2257") {
		http.Error(w, "certify first", http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "no image", http.StatusBadRequest)
		return
	}
	defer f.Close()
	ct := hdr.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		http.Error(w, "not an image", http.StatusBadRequest)
		return
	}
	mediaURL, cfID, err := a.media.StoreImage(r.Context(), f, ct)
	if err != nil {
		http.Error(w, "store failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"cf_id": cfID,
		"url":   mediaURL,
		"thumb": a.media.ThumbURL(cfID),
	})
}

// publishLayout binds the studio composition, fills delivery URLs from the
// Cloudflare ids (client URLs are not trusted), and 303s home.
func (a *App) publishLayout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		http.Redirect(w, r, "/studio?err=1", http.StatusSeeOther)
		return
	}
	if !authx.Agreed(r, "cert2257") {
		http.Redirect(w, r, "/studio?err=2257", http.StatusSeeOther)
		return
	}
	raw := strings.TrimSpace(r.FormValue("layout"))
	if raw == "" {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Redirect(w, r, "/studio?err=1", http.StatusSeeOther)
			return
		}
		raw = string(body)
	}
	l, err := content.ParseLayout([]byte(raw))
	if err != nil || l.Validate() != nil {
		http.Redirect(w, r, "/studio?err=1", http.StatusSeeOther)
		return
	}
	// recompute URLs from ids so a crafted payload can't point the wall elsewhere
	for i := range l.Pieces {
		l.Pieces[i].URL = a.media.FullURL(l.Pieces[i].CFID)
	}
	author, _, _ := a.acct(&beach.Ctx{W: w, R: r})
	a.auth.RecordAgreement(r.Context(), author, authx.Agree2257, store.ClientMeta(r))
	first := l.Pieces[0]
	p, err := a.store.AddLayout(r.Context(), l.JSON(), first.URL, first.CFID, author, store.ClientMeta(r))
	if err != nil {
		http.Redirect(w, r, "/studio?err=1", http.StatusSeeOther)
		return
	}
	a.rep.Emit("post", author)
	a.anl.Track(r, "post_layout", author, p.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// pinPost saves a print's place on the shared board. Any member can rearrange
// — the wall is the square-footage, handed over. Raw (not an Action) because
// board.js is a plain fetch, not Datastar.
func (a *App) pinPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "no such post", http.StatusNotFound)
		return
	}
	var in struct {
		X   float64 `json:"x"`
		Y   float64 `json:"y"`
		Rot float64 `json:"rot"`
		W   float64 `json:"w"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in); err != nil {
		http.Error(w, "bad pin", http.StatusBadRequest)
		return
	}
	pin := content.ClampPin(content.Pin{X: in.X, Y: in.Y, Rot: in.Rot, W: in.W})
	if err := a.store.Pin(r.Context(), id, pin.X, pin.Y, pin.Rot, pin.W); err != nil {
		if err == store.ErrNoPost {
			http.Error(w, "no such post", http.StatusNotFound)
			return
		}
		http.Error(w, "pin failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
