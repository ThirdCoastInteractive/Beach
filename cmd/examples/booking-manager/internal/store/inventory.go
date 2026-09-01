package store

import "context"

// Supply is one stocked item at a property. Low stock = qty <= par.
type Supply struct {
	ID         int64
	PropertyID int64
	Property   string
	Name       string
	Category   string
	Qty        float64
	Par        float64
	Unit       string
}

// Low reports whether the item is at or under its restock level.
func (sp Supply) Low() bool { return sp.Qty <= sp.Par }

// ListSupplies returns supplies, optionally filtered to one property
// (0 = all), grouped by property then category then name.
func (s *Store) ListSupplies(ctx context.Context, propertyID int64) ([]Supply, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sp.id, sp.property_id, p.name, sp.name, sp.category, sp.qty, sp.par, sp.unit
		  FROM bm_supplies sp
		  JOIN bm_properties p ON p.id = sp.property_id
		 WHERE $1 = 0 OR sp.property_id = $1
		 ORDER BY p.name, sp.category, sp.name`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Supply{}
	for rows.Next() {
		var sp Supply
		if err := rows.Scan(&sp.ID, &sp.PropertyID, &sp.Property, &sp.Name,
			&sp.Category, &sp.Qty, &sp.Par, &sp.Unit); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// LowSupplies returns every item at or under par, for the dashboard.
func (s *Store) LowSupplies(ctx context.Context) ([]Supply, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sp.id, sp.property_id, p.name, sp.name, sp.category, sp.qty, sp.par, sp.unit
		  FROM bm_supplies sp
		  JOIN bm_properties p ON p.id = sp.property_id
		 WHERE sp.qty <= sp.par
		 ORDER BY p.name, sp.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Supply{}
	for rows.Next() {
		var sp Supply
		if err := rows.Scan(&sp.ID, &sp.PropertyID, &sp.Property, &sp.Name,
			&sp.Category, &sp.Qty, &sp.Par, &sp.Unit); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// AddSupply inserts one stocked item.
func (s *Store) AddSupply(ctx context.Context, sp Supply) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bm_supplies (property_id, name, category, qty, par, unit)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		sp.PropertyID, sp.Name, sp.Category, sp.Qty, sp.Par, sp.Unit)
	return err
}

// AdjustSupply moves an item's quantity by delta, floored at zero.
func (s *Store) AdjustSupply(ctx context.Context, id int64, delta float64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE bm_supplies SET qty = GREATEST(qty + $2, 0) WHERE id = $1`, id, delta)
	return err
}
