package main

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"time"
)

// dateFmt is the canonical on-disk date format for card scheduling.
const dateFmt = "2006-01-02"

// Card is a single flashcard plus its SM-2 scheduling state. Only Front and
// Back are required when authoring a deck by hand (or via Claude); the
// scheduling fields default sensibly on first load.
type Card struct {
	ID       string  `json:"id"`
	Front    string  `json:"front"`
	Back     string  `json:"back"`
	Ease     float64 `json:"ease,omitempty"`     // ease factor, >= 1.3
	Interval int     `json:"interval,omitempty"` // current interval in days
	Reps     int     `json:"reps,omitempty"`     // successful reviews in a row
	Lapses   int     `json:"lapses,omitempty"`   // times failed after learning
	Due      string  `json:"due,omitempty"`      // YYYY-MM-DD; empty == new
}

// normalize fills in defaults for hand-authored cards so the SM-2 math is
// well-defined. Returns true if anything changed (so we can persist it).
func (c *Card) normalize() bool {
	changed := false
	if c.ID == "" {
		c.ID = newID()
		changed = true
	}
	if c.Ease == 0 {
		c.Ease = 2.5
		changed = true
	}
	return changed
}

// IsDue reports whether the card should appear in a review session run on the
// given day. New cards (no Due) are always due.
func (c *Card) IsDue(today time.Time) bool {
	if c.Due == "" {
		return true
	}
	due, err := time.Parse(dateFmt, c.Due)
	if err != nil {
		return true // corrupt date => surface it for review
	}
	return !due.After(today)
}

// Grade is the learner's self-assessment after seeing the answer.
type Grade int

const (
	GradeAgain Grade = iota // forgot it
	GradeHard               // recalled with serious difficulty
	GradeGood               // recalled correctly
	GradeEasy               // trivial
)

// quality maps a Grade onto the SM-2 0..5 quality scale.
func (g Grade) quality() int {
	switch g {
	case GradeAgain:
		return 2
	case GradeHard:
		return 3
	case GradeGood:
		return 4
	case GradeEasy:
		return 5
	}
	return 4
}

// Review applies the SM-2 algorithm to the card for the given grade, mutating
// its scheduling state and computing the next due date from today.
//
// A grade of Again is a lapse: reps reset and the card is scheduled for the
// next day (and, within a session, requeued immediately by the review model).
func (c *Card) Review(g Grade, today time.Time) {
	c.normalize()
	q := g.quality()

	if q < 3 {
		c.Reps = 0
		c.Lapses++
		c.Interval = 1
	} else {
		switch c.Reps {
		case 0:
			c.Interval = 1
		case 1:
			c.Interval = 6
		default:
			c.Interval = int(math.Round(float64(c.Interval) * c.Ease))
		}
		c.Reps++
	}

	// SM-2 ease adjustment.
	c.Ease += 0.1 - float64(5-q)*(0.08+float64(5-q)*0.02)
	if c.Ease < 1.3 {
		c.Ease = 1.3
	}

	c.Due = today.AddDate(0, 0, c.Interval).Format(dateFmt)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
