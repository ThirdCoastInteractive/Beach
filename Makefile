# the-beach build entry points. Views are .templ components compiled by
# `templ generate` (installed as a Go tool — see the tool directive in go.mod);
# the stylesheet is built by the Tailwind v4 CLI from internal/view/css/input.css.
# Generated *_templ.go files are committed, so plain `go build` works without
# running gen first; run `make gen` after editing any .templ or css file.

.PHONY: gen css build vet test

gen:
	go tool templ generate
	$(MAKE) css

css:
	npx @tailwindcss/cli -i internal/view/css/input.css -o internal/view/static/css/app.css

build: gen
	go build ./...

vet: gen
	go vet ./...

test: build
	go test ./...
