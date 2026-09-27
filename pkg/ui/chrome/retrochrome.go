package chrome

// Retrochrome carries Manifold Thirteen's compact control-room presentation
// onto the shared component vocabulary. It changes styling, never behavior.
func Retrochrome() Definition {
	return Definition{
		ID: "retrochrome", Name: "Retrochrome",
		Stylesheet: "css/retrochrome.css", ThemePreset: "retrochrome",
		Navigation: "top", ProductLayout: "split", Density: "compact",
	}
}
