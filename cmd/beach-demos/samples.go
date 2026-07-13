package main

import (
	"time"

	"github.com/ThirdCoastInteractive/the-beach/pkg/chart"
)

// Deterministic beach-resort demo data for the geo suite. The narrative is the
// same one the specimen uses: a seaside resort looking at where its guests come
// from, what they spend, and where its sister properties sit on the map.

// worldVisitors is the season's guest origins by country (ISO alpha-2).
func worldVisitors() []chart.GeoRegion {
	return []chart.GeoRegion{
		{Code: "US", Value: 9420}, {Code: "CA", Value: 3110}, {Code: "MX", Value: 1240},
		{Code: "GB", Value: 980}, {Code: "DE", Value: 720}, {Code: "FR", Value: 640},
		{Code: "ES", Value: 470}, {Code: "IT", Value: 430}, {Code: "NL", Value: 260},
		{Code: "BR", Value: 410}, {Code: "AR", Value: 180}, {Code: "JP", Value: 350},
		{Code: "KR", Value: 210}, {Code: "CN", Value: 300}, {Code: "IN", Value: 240},
		{Code: "AU", Value: 290}, {Code: "NZ", Value: 110}, {Code: "ZA", Value: 130},
		{Code: "NO", Value: 120}, {Code: "SE", Value: 150}, {Code: "IE", Value: 190},
	}
}

// usViewers is the streaming audience by US state (postal codes accepted).
func usViewers() []chart.GeoRegion {
	return []chart.GeoRegion{
		{Code: "CA", Value: 8400}, {Code: "TX", Value: 6100}, {Code: "FL", Value: 5200},
		{Code: "NY", Value: 4800}, {Code: "IL", Value: 3100}, {Code: "MN", Value: 2900},
		{Code: "WI", Value: 2600}, {Code: "MI", Value: 2400}, {Code: "WA", Value: 2200},
		{Code: "CO", Value: 2050}, {Code: "GA", Value: 1900}, {Code: "NC", Value: 1750},
		{Code: "OH", Value: 1600}, {Code: "PA", Value: 1500}, {Code: "AZ", Value: 1450},
		{Code: "MA", Value: 1300}, {Code: "OR", Value: 1150}, {Code: "TN", Value: 980},
		{Code: "AK", Value: 640}, {Code: "HI", Value: 720}, {Code: "MO", Value: 880},
		{Code: "LA", Value: 760}, {Code: "NV", Value: 700}, {Code: "UT", Value: 610},
	}
}

// usRevenue is merch revenue by US state, a different distribution so the two
// US maps read differently.
func usRevenue() []chart.GeoRegion {
	return []chart.GeoRegion{
		{Code: "CA", Value: 141}, {Code: "NY", Value: 128}, {Code: "TX", Value: 96},
		{Code: "FL", Value: 88}, {Code: "MA", Value: 74}, {Code: "WA", Value: 69},
		{Code: "IL", Value: 61}, {Code: "CO", Value: 58}, {Code: "MN", Value: 52},
		{Code: "OR", Value: 44}, {Code: "GA", Value: 41}, {Code: "NC", Value: 38},
		{Code: "AZ", Value: 33}, {Code: "VA", Value: 31}, {Code: "MI", Value: 29},
		{Code: "PA", Value: 27}, {Code: "WI", Value: 24}, {Code: "HI", Value: 22},
		{Code: "AK", Value: 12}, {Code: "NM", Value: 9},
	}
}

// worldPoints are sister-property bubbles sized by nightly capacity.
func worldPoints() []chart.GeoPoint {
	return []chart.GeoPoint{
		{Name: "Third Coast HQ", Lon: -87.6, Lat: 41.9, Value: 100},
		{Name: "Gulf Cabanas", Lon: -95.4, Lat: 29.0, Value: 62},
		{Name: "Baja Point", Lon: -109.9, Lat: 22.9, Value: 48},
		{Name: "Lisbon Wharf", Lon: -9.1, Lat: 38.7, Value: 55},
		{Name: "Amalfi Deck", Lon: 14.6, Lat: 40.6, Value: 40},
		{Name: "Cape Town Swell", Lon: 18.4, Lat: -33.9, Value: 44},
		{Name: "Byron Bay Hut", Lon: 153.6, Lat: -28.6, Value: 38},
		{Name: "Okinawa Reef", Lon: 127.7, Lat: 26.2, Value: 35},
		{Name: "Reykjavik Cold Plunge", Lon: -21.9, Lat: 64.1, Value: 18},
		{Name: "Rio Boardwalk", Lon: -43.2, Lat: -22.9, Value: 52},
	}
}

// usInsetPoints are markers that fall inside the Alaska and Hawaii inset
// windows plus a couple on the mainland, to show the inset-window projection.
func usInsetPoints() []chart.GeoPoint {
	return []chart.GeoPoint{
		{Name: "Honolulu Lanai", Lon: -157.86, Lat: 21.30, Value: 80},
		{Name: "Anchorage Base Camp", Lon: -149.90, Lat: 61.22, Value: 45},
		{Name: "San Diego Pier", Lon: -117.16, Lat: 32.72, Value: 90},
		{Name: "Miami Cabana", Lon: -80.19, Lat: 25.76, Value: 70},
		{Name: "Chicago HQ", Lon: -87.63, Lat: 41.88, Value: 60},
	}
}

// sunClockCities is a fixed roster with real time zones for the custom sun
// clock. FixedZone entries prove the marker local-time path without depending
// on the host tzdata.
func sunClockCities() []chart.GeoClockCity {
	return []chart.GeoClockCity{
		{Name: "Chicago", Lon: -87.63, Lat: 41.88, TZ: time.FixedZone("CST", -6*3600)},
		{Name: "London", Lon: -0.13, Lat: 51.51, TZ: time.FixedZone("BST", 1*3600)},
		{Name: "Tokyo", Lon: 139.69, Lat: 35.69, TZ: time.FixedZone("JST", 9*3600)},
		{Name: "Sydney", Lon: 151.21, Lat: -33.87, TZ: time.FixedZone("AEST", 10*3600)},
		{Name: "Honolulu", Lon: -157.86, Lat: 21.31, TZ: time.FixedZone("HST", -10*3600)},
		{Name: "Reykjavik", Lon: -21.94, Lat: 64.15, TZ: time.FixedZone("GMT", 0)},
	}
}

// frozenClock is a fixed instant so the "custom cities" clock renders
// identically on every build (a mid-afternoon-UTC time that puts the
// terminator across the Atlantic).
var frozenClock = time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
