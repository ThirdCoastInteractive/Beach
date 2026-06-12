package main

// The board is a fixed ring of tiles. It is static data — the live state
// (who owns what) lives in the ecs.Store as Ownership components keyed by tile
// index. Kind drives what landing on the tile does.

type tileKind int

const (
	kindGo       tileKind = iota // collect salary on pass/land
	kindProperty                 // buyable; pay rent if owned by another
	kindTax                      // pay the bank
	kindChance                   // small random gain/loss
	kindJail                     // just visiting (no jail rules in v1)
	kindFree                     // free parking (nothing)
)

// tile is one square. Price is the buy cost; Rent is charged to a visitor who
// does not own it. Group colors tiles for the board legend.
type tile struct {
	Name  string
	Kind  tileKind
	Price int
	Rent  int
	Group string // color-group label for properties
}

// board is a compact 20-square ring: a playable Monopoly-shaped loop with four
// corners and four color groups. Rents are ~half price, the classic ratio.
var board = []tile{
	{Name: "GO", Kind: kindGo},
	{Name: "Tide Pool Ave", Kind: kindProperty, Price: 60, Rent: 8, Group: "teal"},
	{Name: "Driftwood Chance", Kind: kindChance},
	{Name: "Seagrass Lane", Kind: kindProperty, Price: 80, Rent: 12, Group: "teal"},
	{Name: "Harbor Tax", Kind: kindTax, Price: 75},

	{Name: "Just Visiting", Kind: kindJail},
	{Name: "Boardwalk Blvd", Kind: kindProperty, Price: 120, Rent: 18, Group: "coral"},
	{Name: "Pier Chance", Kind: kindChance},
	{Name: "Lighthouse Row", Kind: kindProperty, Price: 140, Rent: 22, Group: "coral"},
	{Name: "Sandbar Street", Kind: kindProperty, Price: 160, Rent: 26, Group: "coral"},

	{Name: "Free Parking", Kind: kindFree},
	{Name: "Coral Court", Kind: kindProperty, Price: 180, Rent: 30, Group: "amber"},
	{Name: "Dune Chance", Kind: kindChance},
	{Name: "Marina Mews", Kind: kindProperty, Price: 200, Rent: 34, Group: "amber"},
	{Name: "Surf Tax", Kind: kindTax, Price: 100},

	{Name: "Just Visiting", Kind: kindJail},
	{Name: "Sunset Strand", Kind: kindProperty, Price: 240, Rent: 42, Group: "violet"},
	{Name: "Cliffside Chance", Kind: kindChance},
	{Name: "Reef Heights", Kind: kindProperty, Price: 280, Rent: 50, Group: "violet"},
	{Name: "Pearl Point", Kind: kindProperty, Price: 320, Rent: 58, Group: "violet"},
}

// salary is collected for passing or landing on GO.
const salary = 200

// startingCash is each player's opening balance.
const startingCash = 1500

// groupColor maps a property's group label to a design token used for the
// board legend swatch (color via token only).
func groupColor(group string) string {
	switch group {
	case "teal":
		return "var(--primary-6)"
	case "coral":
		return "var(--error-6)"
	case "amber":
		return "var(--warning-6)"
	case "violet":
		return "var(--accent-6)"
	default:
		return "var(--neutral-6)"
	}
}
