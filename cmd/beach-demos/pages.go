package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/ThirdCoastInteractive/Beach/pkg/chart"
)

// pages is the whole demo site, in nav order. index leads; the rest are one
// chart family each.
var pages = []page{
	{Slug: "index.html", Nav: "Gallery", Title: "Geo chart gallery",
		Subtitle: "Server-rendered SVG maps, globes, and a sun clock — each page a self-contained static export. Pick a specimen.",
		Build:    buildIndex},
	{Slug: "world.html", Nav: "World map", Title: "World choropleth",
		Subtitle: "An Equal Earth world map: guest origins by country, the five named color ramps, and a stripped-down custom basemap.",
		Build:    buildWorld},
	{Slug: "us-states.html", Nav: "US states", Title: "US composite map",
		Subtitle: "The 50 states with Alaska and Hawaii in their traditional inset windows, two datasets on two ramps.",
		Build:    buildUSStates},
	{Slug: "interactive.html", Nav: "Interactive", Title: "Interactive drill-down",
		Subtitle: "Wheel-zoom, drag-pan, and click-to-drill on a live map — the click wired to an info panel with no backend.",
		Build:    buildInteractive},
	{Slug: "globes.html", Nav: "Globes", Title: "Orthographic globes",
		Subtitle: "The three globe styles, an auto-rotating themed globe, and an off-center orbit-glance dashboard widget.",
		Build:    buildGlobes, Globe: true},
	{Slug: "sun-clock.html", Nav: "Sun clock", Title: "Day / night sun clock",
		Subtitle: "The daylight terminator, civil twilight, the subsolar sun glyph, and city local times.",
		Build:    buildSunClock},
	{Slug: "points.html", Nav: "Points", Title: "Bubble & point maps",
		Subtitle: "Lon/lat bubbles sized by value on the world plane, and city points routed into the US inset windows.",
		Build:    buildPoints},
}

// --- index -------------------------------------------------------------------

func buildIndex() string {
	card := func(href, title, desc, chartHTML string) string {
		inner := chartBox("210px", chartHTML) +
			fmt.Sprintf(`<p class="demo-card-title">%s</p><p class="demo-card-desc">%s</p>`,
				htmlesc(title), htmlesc(desc))
		return fmt.Sprintf(`<a class="demo-panel demo-card demo-span-4" href="%s">%s</a>`, href, inner)
	}

	worldSpec := render(chart.ChartGeoMapFragment("idx-world", chart.LayoutGeoMap(chart.GeoMapData{
		Regions: worldVisitors(), Unit: "visitors", Ramp: "ocean",
	})))
	usSpec := render(chart.ChartGeoMapFragment("idx-us", chart.LayoutGeoMap(chart.GeoMapData{
		Level: chart.GeoLevelUSStates, Regions: usViewers(), Unit: "viewers", Ramp: "forest",
	})))
	interactSpec := render(chart.ChartGeoMapFragment("idx-interact", chart.LayoutGeoMap(chart.GeoMapData{
		Regions: worldVisitors(), Unit: "visitors", Ramp: "royal",
	})))
	globeSpec := render(chart.ChartGlobeFragment("idx-globe", chart.LayoutGlobe(chart.GlobeData{
		Style: "themed", Regions: worldVisitors(), Unit: "visitors", Ramp: "ember",
		Lon0: -40, Lat0: 20, Graticule: true,
	})))
	clockSpec := render(chart.ChartGeoClockFragment("idx-clock", chart.LayoutGeoClock(chart.GeoClockData{
		Time: frozenClock, ShowTwilight: true,
	})))
	pointsSpec := render(chart.ChartGeoMapFragment("idx-points", chart.LayoutGeoMap(chart.GeoMapData{
		Points: worldPoints(), Unit: "beds", Basemap: &chart.GeoBasemap{Outline: true, MeridianStep: 30, ParallelStep: 30},
	})))

	cards := card("world.html", "World choropleth", "Guest origins by country, five ramps, custom basemaps.", worldSpec) +
		card("us-states.html", "US composite", "50 states with Alaska & Hawaii insets, two datasets.", usSpec) +
		card("interactive.html", "Interactive drill", "Zoom, pan, and click-to-drill into a side panel.", interactSpec) +
		card("globes.html", "Orthographic globes", "Solid, themed, wire, auto-rotate, and orbit glance.", globeSpec) +
		card("sun-clock.html", "Sun clock", "Terminator, twilight, sun glyph, city local times.", clockSpec) +
		card("points.html", "Bubble & point maps", "Value bubbles on the world; city points in US insets.", pointsSpec)

	return grid(cards)
}

// --- world -------------------------------------------------------------------

func buildWorld() string {
	main := render(chart.ChartGeoMapFragment("world-main", chart.LayoutGeoMap(chart.GeoMapData{
		Regions:    worldVisitors(),
		ShowCities: true,
		Unit:       "visitors",
		Ramp:       "ocean",
	})))
	mainPanel := panel("Guest origins — default basemap + megacities", "ocean ramp", "",
		chartBox("min(60vw, 520px)", main))

	// Ramp comparison row: identical data, each named ramp.
	ramps := []string{"ocean", "forest", "ember", "royal", "mono"}
	var tiles strings.Builder
	for _, r := range ramps {
		frag := render(chart.ChartGeoMapFragment("world-ramp-"+r, chart.LayoutGeoMap(chart.GeoMapData{
			Regions: worldVisitors(),
			Unit:    "visitors",
			Ramp:    r,
			Basemap: &chart.GeoBasemap{Outline: true},
		})))
		tiles.WriteString(`<div>` + chartBox("150px", frag) +
			fmt.Sprintf(`<p class="demo-tile-cap">%s</p></div>`, r))
	}
	rampPanel := panel("The five named ramps", "same data, ocean · forest · ember · royal · mono", "",
		`<div class="demo-tile-row">`+tiles.String()+`</div>`)

	// Custom basemap: outline + equator only, no meridians/parallels/named lines.
	custom := render(chart.ChartGeoMapFragment("world-custom", chart.LayoutGeoMap(chart.GeoMapData{
		Regions: worldVisitors(),
		Unit:    "visitors",
		Hue:     150,
		Basemap: &chart.GeoBasemap{Outline: true, Equator: true},
	})))
	customPanel := panel("Custom basemap — outline + equator only", "single-hue default ramp (hue 150)", "",
		chartBox("min(55vw, 460px)", custom))

	return mainPanel + rampPanel + customPanel
}

// --- us states ---------------------------------------------------------------

func buildUSStates() string {
	viewers := render(chart.ChartGeoMapFragment("us-viewers", chart.LayoutGeoMap(chart.GeoMapData{
		Level:      chart.GeoLevelUSStates,
		Regions:    usViewers(),
		ShowCities: true,
		Unit:       "viewers",
		Ramp:       "ocean",
	})))
	revenue := render(chart.ChartGeoMapFragment("us-revenue", chart.LayoutGeoMap(chart.GeoMapData{
		Level:   chart.GeoLevelUSStates,
		Regions: usRevenue(),
		Unit:    "k",
		Ramp:    "ember",
	})))

	return panel("Streaming viewers by state", "ocean ramp · Alaska & Hawaii insets · reference cities", "",
		chartBox("min(60vw, 520px)", viewers)) +
		panel("Merch revenue by state", "ember ramp · a different distribution on the same composite", "",
			chartBox("min(60vw, 520px)", revenue))
}

// --- interactive -------------------------------------------------------------

func buildInteractive() string {
	m := render(chart.ChartGeoMapFragment("drill-map", chart.LayoutGeoMap(chart.GeoMapData{
		Regions:     worldVisitors(),
		Unit:        "visitors",
		Ramp:        "royal",
		Interactive: true,
	})))

	rail := `<aside class="demo-rail" id="drill-rail">` +
		`<h3 id="drill-name">Click a country</h3>` +
		`<p class="demo-rail-code" id="drill-code">wheel to zoom · drag to pan · double-click to reset</p>` +
		`<p class="demo-rail-empty" id="drill-empty">The map dispatches a <code>geomap:region</code> event on every click. This page fills the panel from an inline lookup so the click-to-data pattern is visible with no server.</p>` +
		`<div id="drill-body" style="display:none">` +
		`<div class="demo-stat-row"><span class="demo-stat-label">visitors</span><span class="demo-stat-value" id="drill-value">—</span></div>` +
		`<div class="demo-stat-row"><span class="demo-stat-label">rank</span><span class="demo-stat-value" id="drill-rank">—</span></div>` +
		`<div class="demo-stat-row"><span class="demo-stat-label">share</span><span class="demo-stat-value" id="drill-share">—</span></div>` +
		`<svg class="demo-spark" id="drill-spark" viewBox="0 0 300 46" preserveAspectRatio="none" aria-hidden="true"></svg>` +
		`<p class="demo-tile-cap" style="text-align:left">12-week trend (illustrative)</p>` +
		`</div></aside>`

	layout := `<div class="demo-drill">` + chartBox("min(62vw, 560px)", m) + rail + `</div>`

	note := `<p class="demo-note">In production the click is a Datastar <code>DrillAction</code> — e.g. <code>datastar.Get("/api/geo/%s")</code> baked onto every shape, hitting a real endpoint that patches the panel over SSE. This static page swaps that for an inline listener so the interaction works from disk.</p>`

	return panel("Drill into a region", "click a country to load its card", "", layout+note) + interactiveScript()
}

// interactiveScript is the page-local, no-backend drill listener: it reads the
// bubbling geomap:region event, looks up the value from an inline table, and
// paints the info rail (stat rows + a deterministic sparkline seeded by code).
func interactiveScript() string {
	var data strings.Builder
	data.WriteString("{")
	total := 0.0
	regs := worldVisitors()
	for _, r := range regs {
		total += r.Value
	}
	for i, r := range regs {
		if i > 0 {
			data.WriteString(",")
		}
		fmt.Fprintf(&data, "%q:%g", r.Code, r.Value)
	}
	data.WriteString("}")

	return fmt.Sprintf(`<script>
(function () {
  const DATA = %s;
  const TOTAL = %g;
  const ranked = Object.keys(DATA).sort((a, b) => DATA[b] - DATA[a]);
  const $ = (id) => document.getElementById(id);

  // Deterministic 12-point trend from the region code, drawn as a polyline.
  function spark(code, value) {
    let seed = 0;
    for (const ch of code) seed = (seed * 31 + ch.charCodeAt(0)) %% 9973;
    const n = 12, w = 300, h = 46, pad = 3;
    let d = '';
    for (let i = 0; i < n; i++) {
      seed = (seed * 1103515245 + 12345) & 0x7fffffff;
      const jitter = (seed / 0x7fffffff) - 0.5;
      const base = 0.35 + 0.5 * (i / (n - 1));
      const y = h - pad - (Math.max(0.05, Math.min(0.95, base + jitter * 0.4))) * (h - 2 * pad);
      const x = pad + (i / (n - 1)) * (w - 2 * pad);
      d += (i ? 'L' : 'M') + x.toFixed(1) + ' ' + y.toFixed(1);
    }
    return d;
  }

  document.addEventListener('geomap:region', (e) => {
    const { code, name, hasData } = e.detail || {};
    $('drill-name').textContent = name || code || 'Unknown region';
    $('drill-code').textContent = code ? ('code ' + code + (hasData ? '' : ' · no data')) : 'no code';
    const body = $('drill-body'), empty = $('drill-empty');
    const v = code && (code in DATA) ? DATA[code] : null;
    if (v == null) {
      body.style.display = 'none';
      empty.style.display = '';
      empty.textContent = 'No visitor data for this region — every shape is still clickable and dispatches the event.';
      return;
    }
    empty.style.display = 'none';
    body.style.display = '';
    $('drill-value').textContent = v.toLocaleString();
    $('drill-rank').textContent = '#' + (ranked.indexOf(code) + 1) + ' of ' + ranked.length;
    $('drill-share').textContent = (100 * v / TOTAL).toFixed(1) + '%%';
    const path = spark(code, v);
    $('drill-spark').innerHTML =
      '<path d="' + path + '" fill="none" stroke="var(--color-accent)" stroke-width="2" ' +
      'stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"></path>';
  });
})();
</script>`, data.String(), total)
}

// --- globes ------------------------------------------------------------------

func buildGlobes() string {
	trio := func(id, style, label string) string {
		g := chart.LayoutGlobe(chart.GlobeData{
			Style: style, Regions: worldVisitors(), Unit: "visitors", Ramp: "ocean",
			Lon0: -30, Lat0: 25, Graticule: true,
		})
		frag := render(chart.ChartGlobeFragment(id, g))
		return `<div class="demo-panel demo-span-4">` +
			fmt.Sprintf(`<header class="demo-panel-head"><span class="demo-eyebrow">%s</span></header>`, htmlesc(label)) +
			chartBox("300px", frag) + `</div>`
	}
	// Solid uses no regions (single shaded landmass); themed + wire carry data.
	solid := chart.LayoutGlobe(chart.GlobeData{Style: "solid", Lon0: -30, Lat0: 25, Graticule: true})
	solidFrag := render(chart.ChartGlobeFragment("globe-solid", solid))
	solidCard := `<div class="demo-panel demo-span-4">` +
		`<header class="demo-panel-head"><span class="demo-eyebrow">solid</span></header>` +
		chartBox("300px", solidFrag) + `</div>`

	trioRow := grid(solidCard + trio("globe-themed", "themed", "themed") + trio("globe-wire", "wire", "wire"))
	trioPanel := panel("The three styles", "static server-rendered frames — no JS required", "", trioRow)

	// Auto-rotating themed globe (animated → needs the inline world geometry).
	rotate := chart.LayoutGlobe(chart.GlobeData{
		Style: "themed", Regions: worldVisitors(), Unit: "visitors", Ramp: "royal",
		Lon0: -20, Lat0: 18, RotateSpeed: 12, Graticule: true,
	})
	rotateFrag := render(chart.ChartGlobeFragment("globe-rotate", rotate))

	// Orbit-glance dashboard widget: zoomed and shoved into the corner so only
	// a sweep of the limb shows, with a slow drift.
	orbit := chart.LayoutGlobe(chart.GlobeData{
		Style: "themed", Regions: worldVisitors(), Unit: "visitors", Ramp: "ember",
		Lon0: -60, Lat0: 30, Zoom: 2.4, CenterX: 0.78, CenterY: 0.95,
		RotateSpeed: 4, Orbit: true,
	})
	orbitFrag := render(chart.ChartGlobeFragment("globe-orbit", orbit))

	rotateCard := `<div class="demo-panel demo-span-6">` +
		`<header class="demo-panel-head"><span class="demo-eyebrow">auto-rotate</span><span class="demo-panel-note">12°/s · drag to spin · pauses offscreen</span></header>` +
		chartBox("360px", rotateFrag) + `</div>`
	orbitCard := `<div class="demo-panel demo-span-6">` +
		`<header class="demo-panel-head"><span class="demo-eyebrow">orbit glance</span><span class="demo-panel-note">zoom 2.4 · off-center · slow drift</span></header>` +
		chartBox("360px", orbitFrag) + `</div>`
	animPanel := panel("Animated globes", "these hydrate from the inlined world geometry", "",
		grid(rotateCard+orbitCard))

	note := `<p class="demo-note">Animated globes carry <code>data-globe</code>; <code>globe.js</code> swaps the SSR frame for a canvas that re-projects the country rings each frame. Motion respects <code>prefers-reduced-motion</code> and pauses when the tab is hidden or the globe scrolls out of view.</p>`

	return trioPanel + animPanel + panel("", "", "", note)
}

// --- sun clock ---------------------------------------------------------------

func buildSunClock() string {
	now := time.Now().UTC()
	live := render(chart.ChartGeoClockFragment("clock-live", chart.LayoutGeoClock(chart.GeoClockData{
		Time:         now,
		ShowCities:   true,
		ShowTwilight: true,
	})))
	custom := render(chart.ChartGeoClockFragment("clock-custom", chart.LayoutGeoClock(chart.GeoClockData{
		Time:         frozenClock,
		Cities:       sunClockCities(),
		ShowTwilight: true,
	})))

	livePanel := panel("Generated at "+now.Format("15:04 UTC · Jan 2 2006"),
		"megacities + civil twilight band", "", chartBox("min(58vw, 500px)", live))
	customPanel := panel("Custom city roster", "fixed instant · local times from FixedZone offsets", "",
		chartBox("min(58vw, 500px)", custom))

	note := `<p class="demo-note">A static export freezes the clock at generation time. Live deployments set <code>RefreshAction</code> (a Datastar action) and the fragment re-fetches itself every 60s, morphing the terminator in place — no client script.</p>`

	return livePanel + customPanel + panel("", "", "", note)
}

// --- points ------------------------------------------------------------------

func buildPoints() string {
	world := render(chart.ChartGeoMapFragment("pts-world", chart.LayoutGeoMap(chart.GeoMapData{
		Points:  worldPoints(),
		Unit:    "beds",
		Basemap: &chart.GeoBasemap{Outline: true, MeridianStep: 30, ParallelStep: 30, Equator: true},
	})))
	worldPanel := panel("Sister properties — value bubbles", "bubble area scales with nightly capacity", "",
		chartBox("min(60vw, 520px)", world))

	// US composite with points that fall inside the AK/HI inset windows.
	us := render(chart.ChartGeoMapFragment("pts-us", chart.LayoutGeoMap(chart.GeoMapData{
		Level:   chart.GeoLevelUSStates,
		Points:  usInsetPoints(),
		Unit:    "beds",
		Basemap: &chart.GeoBasemap{MeridianStep: 5, ParallelStep: 5},
	})))
	usPanel := panel("US points routed into the insets", "Honolulu and Anchorage land in their inset windows, not off-map", "",
		chartBox("min(60vw, 520px)", us))

	note := `<p class="demo-note">On the US composite, each point is projected through the same window logic as the state shapes: a lon/lat in the Aleutians or the Hawaiian chain lands inside its inset frame instead of drifting off the conterminous plane.</p>`

	return worldPanel + usPanel + panel("", "", "", note)
}
