// Package locks is booking-manager's smart-lock integration boundary. The app
// programs per-stay door codes through the one small [Provider] interface;
// which lock brand answers is a wiring decision, not an app change. Real
// implementations (August, Schlage Encode, Yale, a Z-Wave bridge) slot in
// behind the same three methods; until one exists the [LogProvider] narrates
// what a real lock would do, so the confirmation flow is exercised end to end
// on a fresh checkout.
package locks

import (
	"context"
	"log/slog"
)

// Status is a lock's last-known state.
type Status struct {
	DeviceID string
	Online   bool
	Battery  int // percent, 0–100
}

// Provider programs guest codes onto a smart lock. DeviceID is the
// provider-native identifier stored on the property; label names the code
// slot (booking-manager uses "guest-<booking id>") so a code can be cleared
// without knowing its digits.
type Provider interface {
	SetCode(ctx context.Context, deviceID, label, code string) error
	ClearCode(ctx context.Context, deviceID, label string) error
	Status(ctx context.Context, deviceID string) (Status, error)
}

// LogProvider prints instead of programming hardware. It is the development
// default: door codes "arrive" in the terminal and Status always reports a
// healthy lock.
type LogProvider struct{}

func (p *LogProvider) SetCode(ctx context.Context, deviceID, label, code string) error {
	slog.Info("locks: not programming", "device", deviceID, "label", label, "code", code)
	return nil
}

func (p *LogProvider) ClearCode(ctx context.Context, deviceID, label string) error {
	slog.Info("locks: not clearing", "device", deviceID, "label", label)
	return nil
}

func (p *LogProvider) Status(ctx context.Context, deviceID string) (Status, error) {
	return Status{DeviceID: deviceID, Online: true, Battery: 100}, nil
}
