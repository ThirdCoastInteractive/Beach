// Command beach-palette derives the framework's color tokens and writes them
// into the Tailwind input stylesheet.
//
// The palette is computed, not picked. Each token declares the backdrops it will
// be seen against and the WCAG ratio it owes there, and the solver in pkg/theme
// returns the color that satisfies all of them with the most of its hue intact
// — so the contrast rules are what *produce* the palette rather than what it is
// graded against afterwards. Changing the entire look, light and dark, is one
// preset key.
//
// Usage:
//
//	beach-palette                  # regenerate input.css from the current preset
//	beach-palette -list            # every preset, with its derived accent
//	beach-palette -serve :7777     # the explorer: gallery, hue wheel, live preview
//	beach-palette -print           # write the block to stdout, touch nothing
//	beach-palette -preset kelp     # derive a different preset (with -print/-serve)
//
// The output replaces the region between the two sentinel comments in
// pkg/beach/view/css/input.css; everything else in the sheet is left alone. Run
// `make css` afterwards to rebuild the served app.css — `make palette` does both.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ThirdCoastInteractive/Beach/pkg/beach/view"
	"github.com/ThirdCoastInteractive/Beach/pkg/theme"
)

// The sentinels bounding the generated region in input.css. They are comments,
// so the sheet stays valid CSS whichever side of a regeneration you read it on.
const (
	beginMark = "/* >>> beach-palette: generated tokens — do not edit by hand */"
	endMark   = "/* <<< beach-palette */"
)

const sheetPath = "pkg/beach/view/css/input.css"

func main() {
	preset := flag.String("preset", view.ThemePreset, "theme preset key to derive from")
	list := flag.Bool("list", false, "list every preset with its derived accent, then exit")
	print := flag.Bool("print", false, "write the generated block to stdout instead of the stylesheet")
	srv := flag.String("serve", "", "serve the theme explorer on this address (e.g. :7777)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: beach-palette [-preset key] [-list] [-print] [-serve addr]")
		flag.PrintDefaults()
	}
	flag.Parse()

	switch {
	case *list:
		listPresets()
		return
	case *srv != "":
		if err := serve(*srv); err != nil {
			fatal(err.Error())
		}
		return
	}

	t, err := theme.BuildPreset(*preset)
	if err != nil {
		fatal(err.Error())
	}
	block := t.CSS()

	if *print {
		fmt.Print(block)
		return
	}

	// Writing the sheet is the one mode where the preset has to match what the
	// framework records, so the stylesheet and the test that re-derives it
	// cannot disagree about which theme is shipping.
	if *preset != view.ThemePreset {
		fatal(fmt.Sprintf("preset %q is not the one the framework records (%q): change view.ThemePreset first",
			*preset, view.ThemePreset))
	}

	src, err := os.ReadFile(sheetPath)
	if err != nil {
		fatal(err.Error())
	}
	out, err := replaceRegion(string(src), block)
	if err != nil {
		fatal(err.Error())
	}
	if out == string(src) {
		fmt.Printf("beach-palette: %s already up to date (%s)\n", sheetPath, t.Preset.Key)
		return
	}
	if err := os.WriteFile(sheetPath, []byte(out), 0o644); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("beach-palette: wrote %d tokens x 2 schemes from preset %q into %s — run `make css`\n",
		len(theme.TokenNames()), t.Preset.Key, sheetPath)
}

// replaceRegion swaps the text between the sentinels for block. Both sentinels
// must already be present: creating them would mean guessing where in the sheet
// the palette belongs, and guessing wrong silently duplicates a :root.
func replaceRegion(src, block string) (string, error) {
	i := strings.Index(src, beginMark)
	j := strings.Index(src, endMark)
	if i < 0 || j < 0 {
		return "", fmt.Errorf("%s is missing the beach-palette sentinels; expected %s … %s", sheetPath, beginMark, endMark)
	}
	if j < i {
		return "", fmt.Errorf("%s has the beach-palette sentinels in the wrong order", sheetPath)
	}
	return src[:i] + beginMark + "\n" + block + src[j:], nil
}

// listPresets prints every preset with what it derives to — the accent it lands
// on in each scheme, which is the fastest way to tell them apart in a terminal.
// The explorer is the way to tell them apart properly.
func listPresets() {
	fmt.Printf("%-12s %-14s %-9s %-9s %s\n", "KEY", "TITLE", "DARK", "LIGHT", "NOTE")
	for _, p := range theme.Presets {
		mark := " "
		if p.Key == view.ThemePreset {
			mark = "*"
		}
		t, err := theme.Build(p.Params)
		if err != nil {
			fmt.Printf("%s%-11s %-14s %s\n", mark, p.Key, p.Title, "— "+err.Error())
			continue
		}
		fmt.Printf("%s%-11s %-14s %-9s %-9s %s\n",
			mark, p.Key, p.Title, t.Dark.Accent.Hex(), t.Light.Accent.Hex(), p.Note)
	}
	fmt.Printf("\n* is the preset the framework currently ships (view.ThemePreset).\n")
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "beach-palette: "+msg)
	os.Exit(1)
}
