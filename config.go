package main

import (
	"fmt"
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

// Ensure creates the directory tree and makes sure at least one field exists.
//
// Notes and decks are organized into fields (subjects) — each field is a
// subdirectory shared by the notes and decks trees. Any loose files left at the
// top level (e.g. from before fields existed) are migrated into a "general"
// field so nothing is hidden once browsing is scoped per field. On a truly
// empty install we seed a "general" field with a sample note and deck.
func (c Config) Ensure() error {
	for _, d := range []string{c.NotesDir, c.DecksDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	migrateLooseFiles(c.NotesDir, defaultField, ".md", ".markdown")
	migrateLooseFiles(c.DecksDir, defaultField, ".json")

	if !c.hasAnyField() {
		general := c.ForField(defaultField)
		for _, d := range []string{general.NotesDir, general.DecksDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		seedSampleNote(general.NotesDir)
		seedSampleDeck(general.DecksDir)
	}
	return nil
}

// defaultField is the field used for seeding and for absorbing loose top-level
// files during migration.
const defaultField = "general"

// ForField returns a Config scoped to a single field's notes and decks
// subdirectories. Sub-models read NotesDir/DecksDir, so scoping a field is just
// a matter of handing them a field-scoped Config.
func (c Config) ForField(field string) Config {
	fc := c
	fc.NotesDir = filepath.Join(c.NotesDir, field)
	fc.DecksDir = filepath.Join(c.DecksDir, field)
	return fc
}

// Field is a study subject: a subdirectory present under the notes and/or decks
// tree, with a count of the notes and decks it holds.
type Field struct {
	Name  string
	Notes int
	Decks int
}

// LoadFields returns the study fields — the union of subdirectories found under
// the notes and decks directories — sorted by name, with per-field counts.
func LoadFields(c Config) ([]Field, error) {
	names := map[string]struct{}{}
	for _, dir := range []string{c.NotesDir, c.DecksDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				names[e.Name()] = struct{}{}
			}
		}
	}

	fields := make([]Field, 0, len(names))
	for name := range names {
		fc := c.ForField(name)
		fields = append(fields, Field{
			Name:  name,
			Notes: countFiles(fc.NotesDir, ".md", ".markdown"),
			Decks: countFiles(fc.DecksDir, ".json"),
		})
	}
	sort.Slice(fields, func(i, j int) bool {
		return strings.ToLower(fields[i].Name) < strings.ToLower(fields[j].Name)
	})
	return fields, nil
}

// CreateField creates a new field by making its subdirectory under both the
// notes and decks trees. It returns the sanitized field name actually used.
func (c Config) CreateField(name string) (string, error) {
	safe := sanitizeField(name)
	if safe == "" {
		return "", fmt.Errorf("invalid field name %q", name)
	}
	for _, dir := range []string{c.NotesDir, c.DecksDir} {
		if err := os.MkdirAll(filepath.Join(dir, safe), 0o755); err != nil {
			return "", err
		}
	}
	return safe, nil
}

// hasAnyField reports whether at least one field subdirectory already exists.
func (c Config) hasAnyField() bool {
	for _, dir := range []string{c.NotesDir, c.DecksDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				return true
			}
		}
	}
	return false
}

// sanitizeField turns a user-entered name into a safe single-segment directory
// name, dropping path separators and leading/trailing dots and spaces.
func sanitizeField(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, string(os.PathSeparator), "-")
	name = strings.Trim(name, ".- ")
	return name
}

// migrateLooseFiles moves any top-level files in dir with one of exts into a
// field subdirectory, so pre-field layouts upgrade cleanly. Existing fields are
// left untouched.
func migrateLooseFiles(dir, field string, exts ...string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var loose []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if matchesExt(e.Name(), exts) {
			loose = append(loose, e.Name())
		}
	}
	if len(loose) == 0 {
		return
	}
	dest := filepath.Join(dir, field)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return
	}
	for _, name := range loose {
		_ = os.Rename(filepath.Join(dir, name), filepath.Join(dest, name))
	}
}

// countFiles counts files (recursively) under dir matching one of exts.
func countFiles(dir string, exts ...string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if matchesExt(d.Name(), exts) {
			n++
		}
		return nil
	})
	return n
}

func matchesExt(name string, exts []string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
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
	"## Fields\n\n" +
	"Notes and flashcards are organized into **fields** — one per subject, e.g.\n" +
	"CyberSecurity or Spanish. termstudy opens on the field picker; press `n` to\n" +
	"create a new field, then `enter` to study it. Each field has its own folder\n" +
	"under `~/.termstudy/notes/<field>` and `~/.termstudy/decks/<field>`.\n\n" +
	"## Workflow with Claude\n\n" +
	"1. Ask Claude to generate study notes as markdown and save them under a\n" +
	"   field's notes folder (e.g. `~/.termstudy/notes/Spanish`).\n" +
	"2. Ask Claude to turn the notes into a flashcard deck — a JSON file in the\n" +
	"   matching `~/.termstudy/decks/<field>` folder, shaped like the sample deck.\n" +
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
