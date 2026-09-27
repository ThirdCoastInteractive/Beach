package theme

// retrochromeParams retains warm equipment-grey surfaces and blue instrument
// accents. Foregrounds and controls are solved by the usual contrast rules.
func retrochromeParams() Params {
	p := params(75, 0.012, 245, 85, 145, 0.85)
	p.Light.PaperL = 0.76
	p.Light.PanelL = 0.87
	p.Light.PanelHoverL = 0.93
	p.Light.LineSoftL = 0.69
	return p
}
