package web

import (
	"regexp"
	"testing"
	"time"

	"github.com/ThirdCoastInteractive/Beach/cmd/examples/booking-manager/internal/store"
)

func TestBuildCalendar(t *testing.T) {
	// July 2026 starts on a Wednesday and ends on a Friday: the grid runs
	// Sun Jun 28 – Sat Aug 1, five weeks.
	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.Local)
	day := func(d int) time.Time { return time.Date(2026, time.July, d, 0, 0, 0, 0, time.Local) }

	weeks := buildCalendar(month, []store.Booking{
		{GuestName: "Ana Whitfield", Status: "confirmed", CheckIn: day(3), CheckOut: day(6)},
		{GuestName: "Tom Reyes", Status: "cancelled", CheckIn: day(3), CheckOut: day(5)},
	})

	if len(weeks) != 5 {
		t.Fatalf("weeks = %d, want 5", len(weeks))
	}
	for _, week := range weeks {
		if len(week) != 7 {
			t.Fatalf("week length = %d, want 7", len(week))
		}
	}
	if got := weeks[0][0]; got.Day != 28 || got.InMonth {
		t.Errorf("first cell = %+v, want Jun 28 out of month", got)
	}
	if got := weeks[0][3]; got.Day != 1 || !got.InMonth {
		t.Errorf("fourth cell = %+v, want Jul 1 in month", got)
	}

	// The stay chips its nights: check-in inclusive, check-out exclusive.
	chipsOn := func(d int) []calChip {
		for _, week := range weeks {
			for _, c := range week {
				if c.InMonth && c.Day == d {
					return c.Chips
				}
			}
		}
		t.Fatalf("day %d not on the grid", d)
		return nil
	}
	for _, d := range []int{3, 4, 5} {
		chips := chipsOn(d)
		if len(chips) != 1 || chips[0].Label != "Ana" {
			t.Errorf("day %d chips = %+v, want one 'Ana' (cancelled stays excluded)", d, chips)
		}
	}
	if chips := chipsOn(6); len(chips) != 0 {
		t.Errorf("check-out day chips = %+v, want none", chips)
	}
}

func TestParseMonth(t *testing.T) {
	got := parseMonth("2026-02")
	if got.Year() != 2026 || got.Month() != time.February || got.Day() != 1 {
		t.Errorf("parseMonth(2026-02) = %v", got)
	}
	// Garbage falls back to the current month, day 1.
	now := time.Now()
	if got := parseMonth("nope"); got.Month() != now.Month() || got.Day() != 1 {
		t.Errorf("parseMonth fallback = %v", got)
	}
}

func TestDollars(t *testing.T) {
	if got := dollars(21500); got != "$215" {
		t.Errorf("dollars(21500) = %q", got)
	}
	if got := dollars(21550); got != "$215.50" {
		t.Errorf("dollars(21550) = %q", got)
	}
}

func TestDoorCode(t *testing.T) {
	four := regexp.MustCompile(`^\d{4}$`)
	for range 20 {
		if code := doorCode(); !four.MatchString(code) {
			t.Fatalf("doorCode() = %q, want 4 digits", code)
		}
	}
}
