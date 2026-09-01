// Package store is pantry's Postgres-backed system of record: inventory,
// storage locations, and shopping lists. pantry requires a database, so this is
// the only implementation (no in-memory fallback). The same handlers that call
// it would, in a generated build, call the sqlc queries; the method surface is
// intentionally narrow so swapping is mechanical.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Domain types. These mirror the columns the annotated sqlc queries in
// internal/db/sql/queries/* select, so a generated db package and these
// hand-written structs would line up field-for-field.

// Item is one inventory row.
type Item struct {
	ID         int64
	Name       string
	Quantity   float64
	Unit       string
	Category   string
	PhotoURL   string
	ExpiresAt  *time.Time
	LocationID int64
	Location   string
}

// Location is a storage place (pantry shelf, fridge, freezer, ...).
type Location struct {
	ID   int64
	Name string
	Kind string
}

// ShoppingList groups list items.
type ShoppingList struct {
	ID    int64
	Name  string
	Items []ListItem
}

// ListItem is one line on a shopping list.
type ListItem struct {
	ID       int64
	Name     string
	Quantity float64
	Checked  bool
}

// Store is the Postgres-backed pantry store — the system of record for
// inventory, locations, and shopping lists.
type Store struct {
	pool *pgxpool.Pool
}

// New returns the store and idempotently seeds a fresh database with the sample
// inventory (3 locations, 6 items, 1 shopping list) so the UI renders something
// on first run. Safe to run on every boot: it only inserts when pantry_items is
// empty.
func New(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	s := &Store{pool: pool}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Pool exposes the underlying pgx pool so callers (e.g. the dashboard widgets)
// can run their own inventory queries without reaching into unexported fields.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) seed(ctx context.Context) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pantry_items`).Scan(&n); err != nil {
		return fmt.Errorf("pantry: seed count: %w", err)
	}
	if n > 0 {
		return nil
	}
	// Locations first; capture ids to place the items.
	var pantry, fridge, freezer int64
	for _, l := range []struct {
		name, kind string
		id         *int64
	}{
		{"Pantry shelf", "pantry", &pantry},
		{"Fridge", "fridge", &fridge},
		{"Freezer", "freezer", &freezer},
	} {
		if err := s.pool.QueryRow(ctx,
			`INSERT INTO pantry_locations (name, kind) VALUES ($1, $2) RETURNING id`,
			l.name, l.kind).Scan(l.id); err != nil {
			return fmt.Errorf("pantry: seed location %q: %w", l.name, err)
		}
	}
	// Items: expiry as current_date + N days (nil for the olive oil).
	type seedItem struct {
		name, unit, cat, photo string
		qty                    float64
		loc                    int64
		expDays                int // 0 = no expiry
	}
	for _, it := range []seedItem{
		{"Whole milk", "L", "dairy", "/static/img/milk.svg", 2, fridge, 5},
		{"Eggs", "doz", "dairy", "/static/img/eggs.svg", 1, fridge, 5},
		{"Pasta", "box", "dry", "/static/img/pasta.svg", 4, pantry, 40},
		{"Olive oil", "bottle", "dry", "/static/img/oil.svg", 1, pantry, 0},
		{"Frozen peas", "bag", "frozen", "/static/img/peas.svg", 3, freezer, 40},
		{"Canned tomatoes", "can", "dry", "/static/img/tomato.svg", 6, pantry, 40},
	} {
		var exp any
		if it.expDays > 0 {
			exp = time.Now().AddDate(0, 0, it.expDays)
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO pantry_items (name, quantity, unit, category, photo_url, location_id, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			it.name, it.qty, it.unit, it.cat, it.photo, it.loc, exp); err != nil {
			return fmt.Errorf("pantry: seed item %q: %w", it.name, err)
		}
	}
	// One shopping list with three lines.
	var listID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO pantry_lists (name) VALUES ('Weekly groceries') RETURNING id`).Scan(&listID); err != nil {
		return fmt.Errorf("pantry: seed list: %w", err)
	}
	for _, li := range []struct {
		name    string
		qty     float64
		checked bool
	}{
		{"Bananas", 6, false},
		{"Bread", 1, true},
		{"Coffee", 1, false},
	} {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO pantry_list_items (list_id, name, quantity, checked) VALUES ($1, $2, $3, $4)`,
			listID, li.name, li.qty, li.checked); err != nil {
			return fmt.Errorf("pantry: seed list item %q: %w", li.name, err)
		}
	}
	return nil
}

// --- reads (deleted_at filtered; ordering in SQL) ---

// ListItems returns the current inventory, soonest-expiring first.
func (s *Store) ListItems(ctx context.Context) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.name, i.quantity, i.unit, i.category, i.photo_url, i.expires_at,
		       COALESCE(i.location_id, 0), COALESCE(l.name, '')
		  FROM pantry_items i
		  LEFT JOIN pantry_locations l ON l.id = i.location_id
		 WHERE i.deleted_at IS NULL
		 ORDER BY (i.expires_at IS NULL), i.expires_at, i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Quantity, &it.Unit, &it.Category,
			&it.PhotoURL, &it.ExpiresAt, &it.LocationID, &it.Location); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListLocations returns the storage locations, alphabetically.
func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, kind FROM pantry_locations WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListLists returns the shopping lists with their lines attached.
func (s *Store) ListLists(ctx context.Context) ([]ShoppingList, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sl.id, sl.name, li.id, li.name, li.quantity, li.checked
		  FROM pantry_lists sl
		  LEFT JOIN pantry_list_items li ON li.list_id = sl.id
		 WHERE sl.deleted_at IS NULL
		 ORDER BY sl.name, li.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShoppingList
	byID := map[int64]int{} // list id -> index in out
	for rows.Next() {
		var (
			lid    int64
			lname  string
			liID   *int64
			liName *string
			liQty  *float64
			liChk  *bool
		)
		if err := rows.Scan(&lid, &lname, &liID, &liName, &liQty, &liChk); err != nil {
			return nil, err
		}
		idx, ok := byID[lid]
		if !ok {
			out = append(out, ShoppingList{ID: lid, Name: lname})
			idx = len(out) - 1
			byID[lid] = idx
		}
		if liID != nil { // LEFT JOIN: skip the null row of an empty list
			out[idx].Items = append(out[idx].Items, ListItem{
				ID: *liID, Name: *liName, Quantity: *liQty, Checked: *liChk,
			})
		}
	}
	return out, rows.Err()
}

// --- writes (callers re-render from a fresh read, so only the error matters) ---

// AddItem inserts one inventory row.
func (s *Store) AddItem(ctx context.Context, it Item) error {
	var loc any
	if it.LocationID != 0 {
		loc = it.LocationID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO pantry_items (name, quantity, unit, category, location_id, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		it.Name, it.Quantity, it.Unit, it.Category, loc, it.ExpiresAt)
	return err
}

// AddLocation inserts one storage location.
func (s *Store) AddLocation(ctx context.Context, name, kind string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO pantry_locations (name, kind) VALUES ($1, $2)`, name, kind)
	return err
}
