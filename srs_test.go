package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustDate(s string) time.Time {
	t, err := time.Parse(dateFmt, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestReviewProgression(t *testing.T) {
	today := mustDate("2026-06-15")
	c := &Card{Front: "q", Back: "a"}

	// First good review schedules 1 day out.
	c.Review(GradeGood, today)
	if c.Interval != 1 || c.Reps != 1 {
		t.Fatalf("after 1st good: interval=%d reps=%d", c.Interval, c.Reps)
	}
	if c.Due != "2026-06-16" {
		t.Fatalf("due = %q, want 2026-06-16", c.Due)
	}

	// Second good review schedules 6 days out.
	c.Review(GradeGood, today)
	if c.Interval != 6 || c.Reps != 2 {
		t.Fatalf("after 2nd good: interval=%d reps=%d", c.Interval, c.Reps)
	}

	// Third good review multiplies by ease (~2.5).
	c.Review(GradeGood, today)
	if c.Interval < 14 || c.Interval > 16 {
		t.Fatalf("after 3rd good: interval=%d, want ~15", c.Interval)
	}
}

func TestAgainResetsAndLapses(t *testing.T) {
	today := mustDate("2026-06-15")
	c := &Card{Front: "q", Back: "a", Reps: 3, Interval: 30, Ease: 2.5}

	c.Review(GradeAgain, today)
	if c.Reps != 0 {
		t.Fatalf("reps = %d, want 0 after Again", c.Reps)
	}
	if c.Lapses != 1 {
		t.Fatalf("lapses = %d, want 1", c.Lapses)
	}
	if c.Interval != 1 || c.Due != "2026-06-16" {
		t.Fatalf("interval=%d due=%q, want 1 / 2026-06-16", c.Interval, c.Due)
	}
	if c.Ease >= 2.5 {
		t.Fatalf("ease = %v, should drop after Again", c.Ease)
	}
}

func TestEaseFloor(t *testing.T) {
	today := mustDate("2026-06-15")
	c := &Card{Front: "q", Back: "a", Ease: 1.3}
	for i := 0; i < 5; i++ {
		c.Review(GradeAgain, today)
	}
	if c.Ease < 1.3 {
		t.Fatalf("ease = %v, must not drop below 1.3", c.Ease)
	}
}

func TestIsDue(t *testing.T) {
	today := mustDate("2026-06-15")
	cases := []struct {
		due  string
		want bool
	}{
		{"", true},               // new card
		{"2026-06-14", true},     // overdue
		{"2026-06-15", true},     // due today
		{"2026-06-16", false},    // future
		{"not-a-date", true},     // corrupt => surface
	}
	for _, tc := range cases {
		c := &Card{Due: tc.due}
		if got := c.IsDue(today); got != tc.want {
			t.Errorf("IsDue(%q) = %v, want %v", tc.due, got, tc.want)
		}
	}
}

func TestDeckRoundTripBackfillsFields(t *testing.T) {
	dir := t.TempDir()
	// Author a deck with only front/back, like Claude would generate.
	raw := `{"name":"T","cards":[{"front":"q1","back":"a1"},{"front":"q2","back":"a2"}]}`
	path := filepath.Join(dir, "t.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	decks, err := LoadDecks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(decks) != 1 || len(decks[0].Cards) != 2 {
		t.Fatalf("unexpected decks: %+v", decks)
	}
	for _, c := range decks[0].Cards {
		if c.ID == "" || c.Ease != 2.5 {
			t.Fatalf("card not normalized: %+v", c)
		}
	}

	// Reload from disk to confirm normalization was persisted.
	reread, _ := os.ReadFile(path)
	var d Deck
	if err := json.Unmarshal(reread, &d); err != nil {
		t.Fatal(err)
	}
	if d.Cards[0].ID == "" {
		t.Fatal("normalized IDs were not saved back to disk")
	}
}
