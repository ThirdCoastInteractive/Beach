# Natural Earth source data for beach-geogen

TopoJSON derived from [Natural Earth](https://www.naturalearthdata.com/) 1:110m
cultural vectors. Natural Earth is public domain: no attribution or license
obligations attach to embedding or redistributing this data.

| File | Natural Earth dataset | Contents |
|---|---|---|
| `world-110m.json` | `ne_110m_admin_0_countries` | 177 countries, keyed by `ISO_A2_EH` |
| `states-provinces-110m.json` | `ne_110m_admin_1_states_provinces` | 51 US states + DC, keyed by `iso_3166_2` |
| `cities-110m.json` | `ne_110m_populated_places` | 243 populated places (point + `SCALERANK` + `POP_MAX`) |

To refresh: download the shapefile from naturalearthdata.com, convert with
`npx -y geo2topo <name>=<file>.shp` (object names must stay `countries`,
`states`, `cities`), drop the result here, then run `make gen-geo`.

`make gen-geo` emits three committed artifacts: `pkg/chart/geodata_gen.go`
(projected Equal Earth paths + gazetteer), `pkg/chart/geodata_rings_gen.go`
(raw quantized lon/lat rings for the 3D globe renderers), and
`pkg/beach/view/static/geo/world-geo.json` (the client-side globe payload,
served from the embedded static tree).

Notes baked into the generator:

- Shapefile fixed-width strings pad with NULs (`Fiji\x00\x00…`); the generator
  strips them.
- `ISO_A2` is `-99` for five countries (France, Norway, and disputed
  territories); `ISO_A2_EH` is populated for all but two, which are kept as
  code-less landmass.
- Coordinates are unquantized (absolute lon/lat, no `transform` block); the
  generator does not implement delta decoding, so keep quantization off when
  regenerating.
