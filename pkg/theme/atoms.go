package theme

// atomsParams is cream paper and charcoal nucleodynamics with cobalt, cadmium
// and viridian accents. Panels sit on the paper (PaperL == PanelL).
func atomsParams() Params {
	p := params(82, 0.016, 255, 55, 155, 0.90)
	p.Light.PaperL = 0.90
	p.Light.PanelL = 0.90
	p.Light.PanelHoverL = 0.86
	p.Light.LineSoftL = 0.82
	p.Light.FgStrongL = 0.18
	p.Light.FgDefaultL = 0.28
	p.Light.SeriesL = 0.52
	p.Light.NeutralChroma = 0.016
	p.Dark.PaperL = 0.20
	p.Dark.PanelL = 0.20
	p.Dark.PanelHoverL = 0.26
	p.Dark.LineSoftL = 0.32
	p.Dark.FgStrongL = 0.92
	p.Dark.FgDefaultL = 0.86
	p.Dark.SeriesL = 0.72
	p.Dark.NeutralChroma = 0.010
	p.WashAlpha = 0.10
	return p
}
