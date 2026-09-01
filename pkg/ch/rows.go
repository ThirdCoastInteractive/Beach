package ch

import (
	"context"
	"fmt"
)

// Rows runs query against conn and scans every result row into a T via the
// driver's struct scanner, returning them as a slice. It is the thin helper that
// feeds chart.Layout* directly — SQL is the query language, this is the whole
// API. A nil Conn returns no rows and no error so ch-optional code paths stay
// branch-free.
//
// Column-to-field mapping follows the driver's `ch` struct tags. Time bucketing
// and aggregation live in the SQL (toStartOfInterval, count, etc.); Rows only
// carries the result home.
func Rows[T any](ctx context.Context, conn Conn, query string, args ...any) ([]T, error) {
	if conn == nil {
		return nil, nil
	}
	rs, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ch: query: %w", err)
	}
	defer rs.Close()

	var out []T
	for rs.Next() {
		var row T
		if err := rs.ScanStruct(&row); err != nil {
			return nil, fmt.Errorf("ch: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, fmt.Errorf("ch: rows: %w", err)
	}
	return out, nil
}
