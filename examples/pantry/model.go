package main

import (
	"sort"
	"sync"
	"time"
)

// Domain types. These mirror the columns the annotated sqlc queries in
// internal/db/sql/queries/* select, so a generated db package and these
// hand-written structs would line up field-for-field.

// Item is one inventory row.
type Item struct {
	ID         int64
	Name       string
	Quantity   float64
	Unit       string
	Category   string
	PhotoURL   string
	ExpiresAt  *time.Time
	LocationID int64
	Location   string
}

// Location is a storage place (pantry shelf, fridge, freezer, ...).
type Location struct {
	ID   int64
	Name string
	Kind string
}

// ShoppingList groups list items.
type ShoppingList struct {
	ID    int64
	Name  string
	Items []ListItem
}

// ListItem is one line on a shopping list.
type ListItem struct {
	ID       int64
	Name     string
	Quantity float64
	Checked  bool
}

// memStore is the in-memory pantry store used when no DATABASE_URL is set, so
// the binary serves the full CRUD surface (minus auth) without Postgres. With a
// database the same handlers would call the generated sqlc queries; the store
// interface is intentionally narrow so swapping is a one-liner.
//
// It is seeded so the home/inventory pages render something on first run.
type memStore struct {
	mu        sync.Mutex
	items     map[int64]*Item
	locations map[int64]*Location
	lists     map[int64]*ShoppingList
	nextID    int64
}

func newMemStore() *memStore {
	s := &memStore{
		items:     map[int64]*Item{},
		locations: map[int64]*Location{},
		lists:     map[int64]*ShoppingList{},
		nextID:    1,
	}
	s.seed()
	return s
}

func (s *memStore) id() int64 {
	v := s.nextID
	s.nextID++
	return v
}

func (s *memStore) seed() {
	pantry := &Location{ID: s.id(), Name: "Pantry shelf", Kind: "pantry"}
	fridge := &Location{ID: s.id(), Name: "Fridge", Kind: "fridge"}
	freezer := &Location{ID: s.id(), Name: "Freezer", Kind: "freezer"}
	for _, l := range []*Location{pantry, fridge, freezer} {
		s.locations[l.ID] = l
	}

	soon := dayPtr(5)
	later := dayPtr(40)
	type seedItem struct {
		name, unit, cat, photo string
		qty                    float64
		loc                    *Location
		exp                    *time.Time
	}
	seeds := []seedItem{
		{"Whole milk", "L", "dairy", "/static/img/milk.svg", 2, fridge, soon},
		{"Eggs", "doz", "dairy", "/static/img/eggs.svg", 1, fridge, soon},
		{"Pasta", "box", "dry", "/static/img/pasta.svg", 4, pantry, later},
		{"Olive oil", "bottle", "dry", "/static/img/oil.svg", 1, pantry, nil},
		{"Frozen peas", "bag", "frozen", "/static/img/peas.svg", 3, freezer, later},
		{"Canned tomatoes", "can", "dry", "/static/img/tomato.svg", 6, pantry, later},
	}
	for _, sd := range seeds {
		it := &Item{
			ID: s.id(), Name: sd.name, Quantity: sd.qty, Unit: sd.unit,
			Category: sd.cat, PhotoURL: sd.photo, ExpiresAt: sd.exp,
			LocationID: sd.loc.ID, Location: sd.loc.Name,
		}
		s.items[it.ID] = it
	}

	list := &ShoppingList{ID: s.id(), Name: "Weekly groceries", Items: []ListItem{
		{ID: s.id(), Name: "Bananas", Quantity: 6},
		{ID: s.id(), Name: "Bread", Quantity: 1, Checked: true},
		{ID: s.id(), Name: "Coffee", Quantity: 1},
	}}
	s.lists[list.ID] = list
}

func dayPtr(days int) *time.Time {
	t := time.Now().AddDate(0, 0, days)
	return &t
}

// --- reads ---

func (s *memStore) listItems() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool {
		// expiring first (nil last), then name
		ei, ej := out[i].ExpiresAt, out[j].ExpiresAt
		switch {
		case ei != nil && ej != nil && !ei.Equal(*ej):
			return ei.Before(*ej)
		case ei != nil && ej == nil:
			return true
		case ei == nil && ej != nil:
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *memStore) getItem(id int64) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok {
		return Item{}, false
	}
	return *it, true
}

func (s *memStore) listLocations() []Location {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Location, 0, len(s.locations))
	for _, l := range s.locations {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *memStore) listLists() []ShoppingList {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ShoppingList, 0, len(s.lists))
	for _, l := range s.lists {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- writes ---

func (s *memStore) addItem(it Item) Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	it.ID = s.id()
	if loc, ok := s.locations[it.LocationID]; ok {
		it.Location = loc.Name
	}
	s.items[it.ID] = &it
	return it
}

func (s *memStore) addLocation(name, kind string) Location {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := &Location{ID: s.id(), Name: name, Kind: kind}
	s.locations[l.ID] = l
	return *l
}

func (s *memStore) addList(name string) ShoppingList {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := &ShoppingList{ID: s.id(), Name: name}
	s.lists[l.ID] = l
	return *l
}
