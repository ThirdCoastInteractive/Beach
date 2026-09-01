# Beach build entry points. Views are .templ components compiled by
# `templ generate` (installed as a Go tool — see the tool directive in go.mod);
# the stylesheet is built by the Tailwind v4 CLI from pkg/beach/view/css/input.css.
# Generated *_templ.go files are committed, so plain `go build` works without
# running gen first; run `make gen` after editing any .templ or css file.

.PHONY: gen css palette build vet test gen-geo notice

gen:
	go tool templ generate
	$(MAKE) css

# Regenerate the geo chart data from the Natural Earth TopoJSON under
# cmd/beach-geogen/data/: pkg/chart/geodata_gen.go (Equal Earth country/state
# paths + city gazetteer), pkg/chart/geodata_rings_gen.go (raw lon/lat rings
# for 3D/orthographic rendering), and pkg/beach/view/static/geo/world-geo.json
# (the client-side globe payload). Only needed when that data or a projection
# changes; all outputs are committed.
gen-geo:
	go run ./cmd/beach-geogen

css:
	npx @tailwindcss/cli -i pkg/beach/view/css/input.css -o pkg/beach/view/static/css/app.css

# Re-derive the design tokens and rebuild the sheet. The palette is computed, not
# picked: every stop is solved against the WCAG ratios it owes (pkg/theme), so
# re-theming the whole framework — both schemes, every token — is changing
# view.ThemePreset and running this. `beach-palette -serve :7777` is the explorer
# for choosing one; `-list` prints them all.
palette:
	go run ./cmd/beach-palette
	$(MAKE) css

build: gen
	go build ./...

vet: gen
	go vet ./...

test: build
	go test ./...

notice:
	go list -m -mod=mod all > /tmp/beach-mods.txt
	{ echo "Beach third-party notices"; echo; echo "Datastar free core — Apache-2.0"; echo "templ, pgx, goose, ClickHouse Go, goldmark, bluemonday, minio-go, mailgun-go, coder/websocket — see their licenses in vendor/"; echo "rybitten Go port — MIT; attribute meodai/RYBitten"; echo "Natural Earth — public domain"; echo "Tailwind CSS — MIT (build-time)"; echo; echo "Go modules:"; cat /tmp/beach-mods.txt; } > NOTICE

# --- example stacks (docker) -------------------------------------------------
# Each example ships a self-contained stack: the app plus its own Postgres and
# ClickHouse (only the web port is published). The up-* targets build the app
# image and bring the stack up detached; down-* stops it (keeping the data
# volumes — add `-v` by hand to drop them). Credentials and the published PORT
# come from cmd/examples/<app>/.env when present (copy .env.example and edit it),
# otherwise compose's built-in defaults (beach/beach, PORT 8080). Running more
# than one stack at once needs a distinct PORT per .env. Deps are vendored.

.PHONY: up-pantry up-driftbottle up-boardwalk up-booking-manager examples-up \
        down-pantry down-driftbottle down-boardwalk down-booking-manager examples-down

up-pantry:
	cd cmd/examples/pantry && docker compose up -d --build

up-driftbottle:
	cd cmd/examples/driftbottle && docker compose up -d --build

up-boardwalk:
	cd cmd/examples/boardwalk && docker compose up -d --build

up-booking-manager:
	cd cmd/examples/booking-manager && docker compose up -d --build

# Bring up the example stacks (each needs a distinct PORT in its .env to
# avoid a host-port clash).
examples-up: up-pantry up-driftbottle up-boardwalk up-booking-manager

down-pantry:
	cd cmd/examples/pantry && docker compose down

down-driftbottle:
	cd cmd/examples/driftbottle && docker compose down

down-boardwalk:
	cd cmd/examples/boardwalk && docker compose down

down-booking-manager:
	cd cmd/examples/booking-manager && docker compose down

examples-down: down-pantry down-driftbottle down-boardwalk down-booking-manager
