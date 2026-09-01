package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/cmd/hush/internal/store"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var turnstileHTTP = &http.Client{Timeout: 4 * time.Second}

// checkTurnstile verifies the widget token. With no secret configured (dev)
// it passes; in production the secret is required at boot, so an empty token
// or a failed siteverify is a hard no.
func (a *App) checkTurnstile(r *http.Request) bool {
	if a.cfg.TurnstileSecret == "" {
		return true
	}
	token := strings.TrimSpace(r.FormValue("cf-turnstile-response"))
	if token == "" {
		return false
	}
	form := url.Values{
		"secret":   {a.cfg.TurnstileSecret},
		"response": {token},
		"remoteip": {store.ClientMeta(r).IP},
	}
	resp, err := turnstileHTTP.PostForm(turnstileVerifyURL, form)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return false
	}
	var out struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal(body, &out) != nil {
		return false
	}
	return out.Success
}

func withBotErr(dest string) string {
	if strings.Contains(dest, "?") {
		return dest + "&err=bot"
	}
	return dest + "?err=bot"
}
