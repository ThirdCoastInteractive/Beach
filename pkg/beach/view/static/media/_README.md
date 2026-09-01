Sample media for the specimen.

`specimen.en.vtt` is a real two-cue caption file, referenced by
`pkg/ui/specimen/samples.go` as `sampleTrackSrc`. It is a served file rather than
an inline `data:` URI because the framework's CSP sets `media-src 'self'` — and a
`<track default>` is the one thing on a `<video preload="none">` the browser
actually fetches, so a fictional path would 404 on every app that mounts the
specimen.

The `<source>` URLs beside it stay fictional on purpose: they are never
requested, which is `preload="none"` demonstrating itself.

This file is named with a leading underscore so `//go:embed view/static` skips
it — the note belongs in the repo, not in every binary.
