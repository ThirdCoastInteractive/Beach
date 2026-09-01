// Package apicompile is a compile harness for beach-apigen's generated output.
// TestCompile generates api.gen.go into this directory from the testdata queries
// and runs `go build` on it, proving the generated handlers compile against the
// REAL beach module API (not just that they parse). The generated file is written
// and removed by the test; only these stubs are committed.
package apicompile

import (
	"context"

	"github.com/ThirdCoastInteractive/Beach/cmd/beach-apigen/testdata/compile/page"
)

// Item is the stub sqlc row type the queries return.
type Item struct {
	ID   int64
	Name string
}

// Queries is the stub sqlc querier the generated handlers close over.
type Queries struct{}

func (*Queries) GetItem(ctx context.Context, id int64) (Item, error) { return Item{}, nil }
func (*Queries) CreateItem(ctx context.Context, arg CreateItemParams) (Item, error) {
	return Item{}, nil
}
func (*Queries) DeleteItem(ctx context.Context, id int64) error { return nil }

// CreateItemParams is the stub sqlc params struct.
type CreateItemParams struct {
	Name       string `json:"name"`
	Quantity   int64  `json:"quantity"`
	LocationID int64  `json:"location_id"`
}

var _ = page.ItemCard
