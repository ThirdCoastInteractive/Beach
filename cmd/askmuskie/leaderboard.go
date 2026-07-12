package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"
)

// lbRow is one leaderboard entry.
type lbRow struct {
	name string
	n    int
}

// topBy returns the top-10 players by a given counter column. col is a fixed
// literal from leaderboardHandler (never user input), so it is safe to splice.
func topBy(ctx context.Context, pool *pgxpool.Pool, col string) []lbRow {
	q := `SELECT u.username, p.` + col + `
	        FROM askmuskie_players p JOIN users u ON u.id = p.user_id
	       WHERE p.` + col + ` > 0
	       ORDER BY p.` + col + ` DESC, u.username
	       LIMIT 10`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []lbRow
	for rows.Next() {
		var r lbRow
		if rows.Scan(&r.name, &r.n) == nil {
			out = append(out, r)
		}
	}
	return out
}

func barChart(rows []lbRow, unit string) chart.BarChart {
	s := chart.HBarSeries{Unit: unit}
	for _, r := range rows {
		s.Series = append(s.Series, chart.HBarEntry{Label: r.name, Value: r.n})
	}
	return chart.LayoutHBar(s)
}
