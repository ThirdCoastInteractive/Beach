// Package store is boardwalk's Postgres persistence lane: a single row holding
// the latest CBOR snapshot of the ecs.Store. The live game runs in memory; this
// is the durable copy the save loop refreshes every few seconds and boot
// restores from. It is the whole database story — one row, overwritten in place.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snap is the Postgres persistence lane: a single row (id=1) holding the latest
// CBOR snapshot of the ecs.Store.
type Snap struct {
	Pool *pgxpool.Pool
}

// Load returns the saved snapshot blob, or (nil, nil) when no snapshot exists
// yet (a fresh database) so the caller seeds a new game instead of restoring.
func (s Snap) Load(ctx context.Context) ([]byte, error) {
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT data FROM boardwalk_snapshot WHERE id = 1`).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Save upserts the single snapshot row with the current store blob and tick.
func (s Snap) Save(ctx context.Context, data []byte, tick int64) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO boardwalk_snapshot (id, data, tick, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE
		SET data = EXCLUDED.data, tick = EXCLUDED.tick, updated_at = now()`, data, tick)
	return err
}
