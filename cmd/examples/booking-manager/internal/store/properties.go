package store

import (
	"context"
	"time"
)

// Property is one rental unit. LockDeviceID names the smart-lock device the
// locks provider programs (empty = no connected lock).
type Property struct {
	ID               int64
	Name             string
	Description      string
	Address          string
	Sleeps           int
	Bedrooms         int
	NightlyRateCents int64
	LockDeviceID     string
	Notes            string
}

// KeyCode is a standing code on a property (lockbox, gate, shed).
type KeyCode struct {
	ID         int64
	PropertyID int64
	Label      string
	Code       string
	Active     bool
	CreatedAt  time.Time
}

// ListProperties returns the properties, alphabetically.
func (s *Store) ListProperties(ctx context.Context) ([]Property, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, address, sleeps, bedrooms, nightly_rate_cents, lock_device_id, notes
		  FROM bm_properties
		 WHERE deleted_at IS NULL
		 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Property{}
	for rows.Next() {
		var p Property
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Address, &p.Sleeps,
			&p.Bedrooms, &p.NightlyRateCents, &p.LockDeviceID, &p.Notes); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProperty returns one property by id.
func (s *Store) GetProperty(ctx context.Context, id int64) (Property, error) {
	var p Property
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, address, sleeps, bedrooms, nightly_rate_cents, lock_device_id, notes
		  FROM bm_properties
		 WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.Address, &p.Sleeps,
			&p.Bedrooms, &p.NightlyRateCents, &p.LockDeviceID, &p.Notes)
	return p, err
}

// AddProperty inserts one property.
func (s *Store) AddProperty(ctx context.Context, p Property) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_properties (name, description, address, sleeps, bedrooms, nightly_rate_cents, lock_device_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.Name, p.Description, p.Address, p.Sleeps, p.Bedrooms, p.NightlyRateCents, p.LockDeviceID)
	return err
}

// ListKeyCodes returns a property's standing codes, newest last.
func (s *Store) ListKeyCodes(ctx context.Context, propertyID int64) ([]KeyCode, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, property_id, label, code, active, created_at
		  FROM bm_key_codes
		 WHERE property_id = $1
		 ORDER BY created_at, id`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KeyCode{}
	for rows.Next() {
		var k KeyCode
		if err := rows.Scan(&k.ID, &k.PropertyID, &k.Label, &k.Code, &k.Active, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// AddKeyCode inserts one standing code.
func (s *Store) AddKeyCode(ctx context.Context, propertyID int64, label, code string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO bm_key_codes (property_id, label, code) VALUES ($1, $2, $3)`,
		propertyID, label, code)
	return err
}

// ToggleKeyCode flips a code's active flag and returns its property id so the
// caller can re-render that property's code list.
func (s *Store) ToggleKeyCode(ctx context.Context, id int64) (int64, error) {
	var propertyID int64
	err := s.pool.QueryRow(ctx,
		`UPDATE bm_key_codes SET active = NOT active WHERE id = $1 RETURNING property_id`, id).
		Scan(&propertyID)
	return propertyID, err
}
