package ecs

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Tick is a monotonic logical clock supplied by the caller (the sim's tick
// loop). The Store does not advance it; every mutation stamps the touched
// column row with the Store's current tick so Changed[T](since) can filter.
type Tick uint64

// column is the non-generic interface the Store holds for each component in an
// archetype. The Store manipulates columns positionally (by row) without
// knowing the element type; typed access happens through the generic helpers
// which type-assert to *typedColumn[T].
type column interface {
	// len reports the number of rows.
	len() int
	// appendZero grows the column by one zero-valued row stamped at tick and
	// returns the new row index. Used when an entity enters this archetype.
	appendZero(tick Tick) int
	// appendFrom copies row src from another column of the same component type
	// into this column, preserving its stored tick, and returns the new row
	// index. Used when an entity migrates between archetypes.
	appendFrom(src column, srcRow int) int
	// swapRemove removes row by swapping the last row into its place (O(1),
	// order is not significant in SoA storage) and returns the entity-relevant
	// info the caller needs: nothing here; the Store fixes up its row->entity
	// map separately.
	swapRemove(row int)
	// ticks returns the per-row last-write tick slice (aliased; snapshot
	// restore writes through it to reinstate historical ticks).
	ticks() []Tick
	// marshalValues encodes the column's data slice as a standalone CBOR value
	// (a typed array). Used by snapshots.
	marshalValues() ([]byte, error)
	// unmarshalValues decodes a CBOR array (as produced by marshalValues) into
	// this column, replacing its contents with n rows (ticks left zero).
	unmarshalValues(data []byte, n int) error
	// copyRowFrom copies row src from another column of the same type into this
	// column's existing row dst (in place, not appending).
	copyRowFrom(dst int, src column, srcRow int) error
}

// typedColumn is the concrete SoA storage for one component type T: a slice of
// values and a parallel slice of last-write ticks.
type typedColumn[T any] struct {
	data  []T
	stamp []Tick
}

func newTypedColumn[T any]() *typedColumn[T] {
	return &typedColumn[T]{}
}

func (c *typedColumn[T]) len() int { return len(c.data) }

func (c *typedColumn[T]) appendZero(tick Tick) int {
	var zero T
	c.data = append(c.data, zero)
	c.stamp = append(c.stamp, tick)
	return len(c.data) - 1
}

func (c *typedColumn[T]) appendFrom(src column, srcRow int) int {
	s := src.(*typedColumn[T])
	c.data = append(c.data, s.data[srcRow])
	c.stamp = append(c.stamp, s.stamp[srcRow])
	return len(c.data) - 1
}

func (c *typedColumn[T]) swapRemove(row int) {
	last := len(c.data) - 1
	c.data[row] = c.data[last]
	c.stamp[row] = c.stamp[last]
	var zero T
	c.data[last] = zero // drop reference so GC can reclaim
	c.data = c.data[:last]
	c.stamp = c.stamp[:last]
}

func (c *typedColumn[T]) ticks() []Tick { return c.stamp }

func (c *typedColumn[T]) marshalValues() ([]byte, error) {
	return cbor.Marshal(c.data)
}

func (c *typedColumn[T]) unmarshalValues(data []byte, n int) error {
	var vals []T
	if err := cbor.Unmarshal(data, &vals); err != nil {
		return err
	}
	if len(vals) != n {
		return fmt.Errorf("ecs: column decode got %d values, want %d", len(vals), n)
	}
	c.data = vals
	c.stamp = make([]Tick, n)
	return nil
}

func (c *typedColumn[T]) copyRowFrom(dst int, src column, srcRow int) error {
	c.data[dst] = src.(*typedColumn[T]).data[srcRow]
	return nil
}
