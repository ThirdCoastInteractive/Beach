package game

// The board is a fixed ring of tiles. It is static data — the live state
// (who owns what) lives in the ecs.Store as Ownership components keyed by tile
// index. Kind drives what landing on the tile does.

// TileKind enumerates what landing on a tile does.
type TileKind int

const (
	KindGo       TileKind = iota // collect salary on pass/land
	KindProperty                 // buyable; pay rent if owned by another
	KindTax                      // pay the bank
	KindChance                   // small random gain/loss
	KindJail                     // just visiting (no jail rules in v1)
	KindFree                     // free parking (nothing)
)

// Tile is one square. Price is the buy cost; Rent is charged to a visitor who
// does not own it. Group colors tiles for the board legend.
type Tile struct {
	Name  string
	Kind  TileKind
	Price int
	Rent  int
	Group string // color-group label for properties
}

// MaxSeats is the fixed table size. Seats fill as players join; unclaimed seats
// render as open. Keeping it small keeps the board legible.
const MaxSeats = 4

// Tiles is a compact 20-square ring: a playable Monopoly-shaped loop with four
// corners and four color groups. Rents are ~half price, the classic ratio.
var Tiles = []Tile{
	{Name: "GO", Kind: KindGo},
	{Name: "Tide Pool Ave", Kind: KindProperty, Price: 60, Rent: 8, Group: "teal"},
	{Name: "Driftwood Chance", Kind: KindChance},
	{Name: "Seagrass Lane", Kind: KindProperty, Price: 80, Rent: 12, Group: "teal"},
	{Name: "Harbor Tax", Kind: KindTax, Price: 75},

	{Name: "Just Visiting", Kind: KindJail},
	{Name: "Boardwalk Blvd", Kind: KindProperty, Price: 120, Rent: 18, Group: "coral"},
	{Name: "Pier Chance", Kind: KindChance},
	{Name: "Lighthouse Row", Kind: KindProperty, Price: 140, Rent: 22, Group: "coral"},
	{Name: "Sandbar Street", Kind: KindProperty, Price: 160, Rent: 26, Group: "coral"},

	{Name: "Free Parking", Kind: KindFree},
	{Name: "Coral Court", Kind: KindProperty, Price: 180, Rent: 30, Group: "amber"},
	{Name: "Dune Chance", Kind: KindChance},
	{Name: "Marina Mews", Kind: KindProperty, Price: 200, Rent: 34, Group: "amber"},
	{Name: "Surf Tax", Kind: KindTax, Price: 100},

	{Name: "Just Visiting", Kind: KindJail},
	{Name: "Sunset Strand", Kind: KindProperty, Price: 240, Rent: 42, Group: "violet"},
	{Name: "Cliffside Chance", Kind: KindChance},
	{Name: "Reef Heights", Kind: KindProperty, Price: 280, Rent: 50, Group: "violet"},
	{Name: "Pearl Point", Kind: KindProperty, Price: 320, Rent: 58, Group: "violet"},
}

// Salary is collected for passing or landing on GO.
const Salary = 200

// StartingCash is each player's opening balance.
const StartingCash = 1500

// GroupColor maps a property's group label to a series token used for the
// board legend swatch (color via token only). The four groups are spread
// evenly around the 15-step qualitative series palette so they read as
// distinct hues whatever gamut is active.
func GroupColor(group string) string {
	switch group {
	case "teal":
		return "var(--color-series-a)"
	case "coral":
		return "var(--color-series-e)"
	case "amber":
		return "var(--color-series-i)"
	case "violet":
		return "var(--color-series-m)"
	default:
		return "var(--color-line-strong)"
	}
}
