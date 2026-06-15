package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Deck is a named collection of cards, backed by a single JSON file. Authoring
// a deck (by hand or with Claude) only requires the name and cards' front/back;
// everything else is filled in and persisted on first load.
type Deck struct {
	Name  string  `json:"name"`
	Cards []*Card `json:"cards"`

	path string // source file; not serialized
}

// DueCount returns how many cards are due for review on the given day.
func (d *Deck) DueCount(today time.Time) int {
	n := 0
	for _, c := range d.Cards {
		if c.IsDue(today) {
			n++
		}
	}
	return n
}

// DueCards returns the cards due on the given day, in file order.
func (d *Deck) DueCards(today time.Time) []*Card {
	var out []*Card
	for _, c := range d.Cards {
		if c.IsDue(today) {
			out = append(out, c)
		}
	}
	return out
}

// LoadDecks reads every *.json deck in dir, normalizes hand-authored cards, and
// persists any backfilled fields so the files stay self-consistent.
func LoadDecks(dir string) ([]*Deck, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var decks []*Deck
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		d, err := loadDeck(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		decks = append(decks, d)
	}
	sort.Slice(decks, func(i, j int) bool {
		return strings.ToLower(decks[i].Name) < strings.ToLower(decks[j].Name)
	})
	return decks, nil
}

func loadDeck(path string) (*Deck, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Deck
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	d.path = path
	if d.Name == "" {
		base := filepath.Base(path)
		d.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	changed := false
	for _, c := range d.Cards {
		if c.normalize() {
			changed = true
		}
	}
	if changed {
		_ = d.Save()
	}
	return &d, nil
}

// CreateDeck creates a new empty deck named name as a JSON file in dir and
// returns the loaded deck, ready for cards to be added. The name doubles as the
// filename (sanitized, with a .json extension). It errors on a blank name or if
// a deck file with that name already exists.
func CreateDeck(dir, name string) (*Deck, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return nil, fmt.Errorf("invalid deck name %q", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sanitizeDeckFile(clean))
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("deck %q already exists", filepath.Base(path))
	}
	d := &Deck{Name: clean, Cards: []*Card{}, path: path}
	if err := d.Save(); err != nil {
		return nil, err
	}
	return d, nil
}

// sanitizeDeckFile derives a safe single-segment <name>.json filename from a
// deck name.
func sanitizeDeckFile(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, string(os.PathSeparator), "-")
	name = strings.Trim(name, ".- ")
	return name + ".json"
}

// AddCard appends a new hand-authored card (front/back only) to the deck,
// backfills its scheduling defaults, and persists the deck to disk.
func (d *Deck) AddCard(front, back string) error {
	c := &Card{Front: strings.TrimSpace(front), Back: strings.TrimSpace(back)}
	c.normalize()
	d.Cards = append(d.Cards, c)
	return d.Save()
}

// UpdateCard replaces a card's front/back (trimmed) and persists the deck. Only
// the faces change; the card's SM-2 scheduling state is left intact.
func (d *Deck) UpdateCard(c *Card, front, back string) error {
	front = strings.TrimSpace(front)
	back = strings.TrimSpace(back)
	if front == "" || back == "" {
		return fmt.Errorf("both front and back are required")
	}
	c.Front = front
	c.Back = back
	return d.Save()
}

// RemoveCard deletes card c from the deck and persists the change.
func (d *Deck) RemoveCard(c *Card) error {
	for i, card := range d.Cards {
		if card == c {
			d.Cards = append(d.Cards[:i], d.Cards[i+1:]...)
			return d.Save()
		}
	}
	return fmt.Errorf("card not found in deck")
}

// Rename changes the deck's display name and persists it. The backing filename
// is left unchanged; decks are identified in the UI by their name field.
func (d *Deck) Rename(name string) error {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return fmt.Errorf("invalid deck name %q", name)
	}
	d.Name = clean
	return d.Save()
}

// Delete removes the deck's backing file from disk.
func (d *Deck) Delete() error {
	if d.path == "" {
		return fmt.Errorf("deck %q has no backing file", d.Name)
	}
	return os.Remove(d.path)
}

// Save writes the deck back to its source file as pretty JSON.
func (d *Deck) Save() error {
	if d.path == "" {
		return fmt.Errorf("deck %q has no backing file", d.Name)
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.path)
}
