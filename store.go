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
