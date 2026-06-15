package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config holds the resolved locations termstudy reads from. Defaults live under
// ~/.termstudy and can be overridden with flags.
type Config struct {
	Root     string // ~/.termstudy
	NotesDir string // markdown notes
	DecksDir string // JSON flashcard decks
}

// DefaultConfig resolves the standard layout, allowing flag overrides for the
// notes and decks directories.
func DefaultConfig(notesOverride, decksOverride string) Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	root := filepath.Join(home, ".termstudy")
	c := Config{
		Root:     root,
		NotesDir: filepath.Join(root, "notes"),
		DecksDir: filepath.Join(root, "decks"),
	}
	if notesOverride != "" {
		c.NotesDir, _ = filepath.Abs(notesOverride)
	}
	if decksOverride != "" {
		c.DecksDir, _ = filepath.Abs(decksOverride)
	}
	return c
}

// Ensure creates the directory tree, seeding a sample note and deck the first
// time so the app is usable immediately.
func (c Config) Ensure() error {
	for _, d := range []string{c.NotesDir, c.DecksDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	seedSampleNote(c.NotesDir)
	seedSampleDeck(c.DecksDir)
	return nil
}

// Note is a markdown file discovered under the notes directory.
type Note struct {
	Title string // path relative to notes dir
	Path  string // absolute path
}

// LoadNotes walks the notes directory and returns every markdown file, sorted
// by their relative path.
func LoadNotes(dir string) ([]Note, error) {
	var notes []Note
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		notes = append(notes, Note{Title: rel, Path: path})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool {
		return strings.ToLower(notes[i].Title) < strings.ToLower(notes[j].Title)
	})
	return notes, nil
}

func seedSampleNote(dir string) {
	p := filepath.Join(dir, "welcome.md")
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.WriteFile(p, []byte(sampleNote), 0o644)
}

func seedSampleDeck(dir string) {
	p := filepath.Join(dir, "sample.json")
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.WriteFile(p, []byte(sampleDeck), 0o644)
}

const sampleNote = "# Welcome to termstudy\n\n" +
	"This is a markdown note. Browse notes in the **Notes** screen, press `e`\n" +
	"to open the selected note in your `$EDITOR` (vim), and it reloads when you\n" +
	"return.\n\n" +
	"## Workflow with Claude\n\n" +
	"1. Ask Claude to generate study notes as markdown and save them in this\n" +
	"   directory (`~/.termstudy/notes`).\n" +
	"2. Ask Claude to turn the notes into a flashcard deck — a JSON file in\n" +
	"   `~/.termstudy/decks` shaped like the sample deck.\n" +
	"3. Review the cards in the **Review** screen; scheduling is handled for you.\n\n" +
	"## tmux tip\n\n" +
	"Run `termstudy` in one pane and keep `vim` open in another for note taking.\n"

const sampleDeck = `{
  "name": "Sample Deck",
  "cards": [
    {
      "front": "What spaced-repetition algorithm does termstudy use?",
      "back": "SM-2, the algorithm popularized by SuperMemo and Anki."
    },
    {
      "front": "How do you grade a card during review?",
      "back": "Press 1 (Again), 2 (Hard), 3 (Good), or 4 (Easy)."
    },
    {
      "front": "Where do flashcard decks live?",
      "back": "As JSON files in ~/.termstudy/decks — one file per deck."
    }
  ]
}
`
