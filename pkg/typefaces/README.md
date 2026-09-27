# Fonts for web and captions

`typefaces` brings Rewind's Google Fonts workflow into Beach without global
configuration. A `Library` owns a local directory. Search uses the Google Fonts
catalog distributed by Fontsource; installation downloads available Latin normal
400/700 WOFF2 and TTF faces. Rendering and caption export use the installed files
without a network request. No new Go dependencies are required.

```sh
go run ./cmd/beach-fonts search condensed
go run ./cmd/beach-fonts -dir static/fonts install "Barlow Condensed"
go run ./cmd/beach-fonts -dir static/fonts list
go run ./cmd/beach-fonts -dir static/fonts css
go run ./cmd/beach-fonts -dir static/fonts file "Barlow Condensed"
```

Flags precede the command. `css` prints same-origin `@font-face` declarations;
save them in your app stylesheet and serve the installed directory at the matching
`-prefix` (default `/static/fonts`). Include the applicable font license when
shipping downloaded assets; the installer downloads font binaries and metadata,
not license documents. Commit assets when the app needs to run entirely offline.

```go
fonts, err := typefaces.New("static/fonts", nil)
// Handle errors at each step.
matches, err := fonts.Search(ctx, "condensed", "sans-serif", 20)
face, err := fonts.Install(ctx, "Barlow Condensed")
captionFont, err := fonts.File(face.Family, 700) // absolute TTF path, nearest weight
css, err := fonts.CSS("/static/fonts")
```

`Installed` lists completed families. `Open` implements `http.FileSystem` for
font files only, excluding metadata and directory listings. Downloads are size
bounded, validate their format signature, and publish only after completion.
The injected HTTP client makes provider requests testable. `Popular`, `Blackletter`,
`NormalizeCategory`, and `GoogleCSSURL` support font pickers and optional remote
previews. Remote previews contact Google; shipped chromes use local assets.

Use `typefaces.Roles{Display, Body, Caption, Label, Mono}` on a chrome definition
and list bundled WOFF2 files in `Fonts: []typefaces.WebFace{...}`. Beach validates
those files at startup and serves scoped typography CSS. Components select roles
without knowing which family is active:

```templ
@components.Text(components.TextProps{
    Role: components.TextCaption,
    Value: "Recorded at 14:32 · North aisle",
})
```

Display headings and product stages use the display role automatically. Body,
caption, label and monospace roles remain readable at their normal text sizes.
