# the-beach build entry points. Views are .templ components compiled by
# `templ generate` (installed as a Go tool — see the tool directive in go.mod);
# the stylesheet is built by the Tailwind v4 CLI from pkg/beach/view/css/input.css.
# Generated *_templ.go files are committed, so plain `go build` works without
# running gen first; run `make gen` after editing any .templ or css file.

.PHONY: gen css palette build vet test gen-geo demos

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

# Writes the static geo-chart demo site to demos/ — self-contained HTML files
# (stylesheet, JS modules, and globe geometry inlined) that open from disk with
# no server. Regenerate after changing the geo charts or their assets.
demos:
	go run ./cmd/beach-demos

build: gen
	go build ./...

vet: gen
	go vet ./...

test: build
	go test ./...

# --- example stacks (docker) -------------------------------------------------
# Each example ships a self-contained stack: the app plus its own Postgres and
# ClickHouse (only the web port is published). The up-* targets build the app
# image and bring the stack up detached; down-* stops it (keeping the data
# volumes — add `-v` by hand to drop them). Credentials and the published PORT
# come from cmd/examples/<app>/.env (cmd/hush/.env for hush) when present (copy .env.example and edit it),
# otherwise compose's built-in defaults (beach/beach, PORT 8080). Running more
# than one stack at once needs a distinct PORT per .env. Deps are vendored, so
# every example (hush included) builds with no SSH/private access; hush just also
# needs the full stack configured (Cloudflare R2/Images + a mail transport) in
# its .env or it exits at boot.

.PHONY: up-pantry up-driftbottle up-boardwalk up-booking-manager up-hush examples-up \
        down-pantry down-driftbottle down-boardwalk down-booking-manager down-hush examples-down

up-pantry:
	cd cmd/examples/pantry && docker compose up -d --build

up-driftbottle:
	cd cmd/examples/driftbottle && docker compose up -d --build

up-boardwalk:
	cd cmd/examples/boardwalk && docker compose up -d --build

up-booking-manager:
	cd cmd/examples/booking-manager && docker compose up -d --build

up-hush:
	cd cmd/hush && docker compose up -d --build

# Bring up the public example stacks (each needs a distinct PORT in its
# .env to avoid a host-port clash). hush is excluded — it needs real Cloudflare
# + mail config in its .env to boot, so start it with `make up-hush`.
examples-up: up-pantry up-driftbottle up-boardwalk up-booking-manager

down-pantry:
	cd cmd/examples/pantry && docker compose down

down-driftbottle:
	cd cmd/examples/driftbottle && docker compose down

down-boardwalk:
	cd cmd/examples/boardwalk && docker compose down

down-booking-manager:
	cd cmd/examples/booking-manager && docker compose down

down-hush:
	cd cmd/hush && docker compose down

examples-down: down-pantry down-driftbottle down-boardwalk down-booking-manager down-hush
