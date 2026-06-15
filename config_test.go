package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	return Config{
		Root:     root,
		NotesDir: filepath.Join(root, "notes"),
		DecksDir: filepath.Join(root, "decks"),
	}
}

func TestEnsureSeedsGeneralField(t *testing.T) {
	c := testConfig(t)
	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}

	fields, err := LoadFields(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0].Name != defaultField {
		t.Fatalf("fields = %+v, want a single %q field", fields, defaultField)
	}
	if fields[0].Notes != 1 || fields[0].Decks != 1 {
		t.Fatalf("seeded counts = %d notes / %d decks, want 1 / 1", fields[0].Notes, fields[0].Decks)
	}
}

func TestEnsureMigratesLooseFiles(t *testing.T) {
	c := testConfig(t)
	// Simulate a pre-fields layout: loose files at the top level.
	for _, d := range []string{c.NotesDir, c.DecksDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(c.NotesDir, "old.md"), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.DecksDir, "old.json"), []byte(`{"name":"O","cards":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}

	// Loose files should now live under the general field, not the top level.
	if _, err := os.Stat(filepath.Join(c.NotesDir, "old.md")); !os.IsNotExist(err) {
		t.Fatal("loose note was not migrated out of the top level")
	}
	if _, err := os.Stat(filepath.Join(c.NotesDir, defaultField, "old.md")); err != nil {
		t.Fatalf("migrated note missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.DecksDir, defaultField, "old.json")); err != nil {
		t.Fatalf("migrated deck missing: %v", err)
	}

	// Migration must not also seed sample content over the real files.
	fields, _ := LoadFields(c)
	if len(fields) != 1 || fields[0].Notes != 1 || fields[0].Decks != 1 {
		t.Fatalf("after migration fields = %+v, want one field with the migrated files only", fields)
	}
}

func TestCreateFieldAndScope(t *testing.T) {
	c := testConfig(t)
	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}

	name, err := c.CreateField("CyberSecurity")
	if err != nil {
		t.Fatal(err)
	}
	if name != "CyberSecurity" {
		t.Fatalf("created field name = %q, want CyberSecurity", name)
	}

	// The field exists under both trees and is scoped correctly.
	fc := c.ForField(name)
	if got := fc.NotesDir; got != filepath.Join(c.NotesDir, name) {
		t.Fatalf("scoped notes dir = %q", got)
	}
	for _, d := range []string{fc.NotesDir, fc.DecksDir} {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			t.Fatalf("field dir %q missing: %v", d, err)
		}
	}

	// It shows up alongside the seeded general field.
	fields, _ := LoadFields(c)
	if len(fields) != 2 {
		t.Fatalf("fields = %+v, want general + CyberSecurity", fields)
	}
}

func TestSanitizeField(t *testing.T) {
	cases := map[string]string{
		"Spanish":          "Spanish",
		"  Spanish  ":      "Spanish",
		"foo/bar":          "foo-bar",
		"../escape":        "escape",
		".":                "",
		"":                 "",
		"Intro to Crypto":  "Intro to Crypto",
	}
	for in, want := range cases {
		if got := sanitizeField(in); got != want {
			t.Errorf("sanitizeField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateFieldRejectsEmpty(t *testing.T) {
	c := testConfig(t)
	if _, err := c.CreateField("   "); err == nil {
		t.Fatal("expected error for blank field name")
	}
}

func TestCreateNote(t *testing.T) {
	dir := t.TempDir()

	path, err := CreateNote(dir, "TCP handshake")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "TCP handshake.md"); path != want {
		t.Fatalf("note path = %q, want %q", path, want)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), "# TCP handshake\n\n"; got != want {
		t.Fatalf("note body = %q, want %q", got, want)
	}

	// It is discoverable as a note.
	notes, err := LoadNotes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Title != "TCP handshake.md" {
		t.Fatalf("LoadNotes = %+v, want the created note", notes)
	}

	// Re-creating the same note is an error rather than an overwrite.
	if _, err := CreateNote(dir, "TCP handshake.md"); err == nil {
		t.Fatal("expected error creating a duplicate note")
	}
}

func TestCreateNoteRejectsBlank(t *testing.T) {
	if _, err := CreateNote(t.TempDir(), "   "); err == nil {
		t.Fatal("expected error for blank note name")
	}
}

func TestRenameField(t *testing.T) {
	c := testConfig(t)
	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}
	// Put a note in the field so we can confirm contents move with it.
	if _, err := CreateNote(c.ForField(defaultField).NotesDir, "keep"); err != nil {
		t.Fatal(err)
	}

	name, err := c.RenameField(defaultField, "Basics")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Basics" {
		t.Fatalf("renamed field = %q, want Basics", name)
	}
	if _, err := os.Stat(filepath.Join(c.NotesDir, defaultField)); !os.IsNotExist(err) {
		t.Fatal("old field dir still present after rename")
	}
	if _, err := os.Stat(filepath.Join(c.NotesDir, "Basics", "keep.md")); err != nil {
		t.Fatalf("note did not move with the field: %v", err)
	}
}

func TestRenameFieldRejectsCollision(t *testing.T) {
	c := testConfig(t)
	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateField("Spanish"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenameField(defaultField, "Spanish"); err == nil {
		t.Fatal("expected error renaming onto an existing field")
	}
}

func TestDeleteField(t *testing.T) {
	c := testConfig(t)
	if err := c.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateField("Temp"); err != nil {
		t.Fatal(err)
	}

	if err := c.DeleteField("Temp"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{c.NotesDir, c.DecksDir} {
		if _, err := os.Stat(filepath.Join(dir, "Temp")); !os.IsNotExist(err) {
			t.Fatalf("field dir %q survived deletion", filepath.Join(dir, "Temp"))
		}
	}
}

func TestRenameAndDeleteNote(t *testing.T) {
	dir := t.TempDir()
	path, err := CreateNote(dir, "old")
	if err != nil {
		t.Fatal(err)
	}

	newPath, err := RenameNote(path, "new title")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "new title.md"); newPath != want {
		t.Fatalf("renamed note = %q, want %q", newPath, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("old note path still exists after rename")
	}

	// Renaming onto an existing note is refused.
	if _, err := CreateNote(dir, "taken"); err != nil {
		t.Fatal(err)
	}
	if _, err := RenameNote(newPath, "taken"); err == nil {
		t.Fatal("expected error renaming onto an existing note")
	}

	if err := DeleteNote(newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("note still exists after delete")
	}
}

func TestSanitizeNoteName(t *testing.T) {
	cases := map[string]string{
		"Intro":          "Intro.md",
		"Intro.md":       "Intro.md",
		"notes.markdown": "notes.markdown",
		"foo/bar":        "foo-bar.md",
		"../escape":      "escape.md",
		"   ":            "",
		"":               "",
	}
	for in, want := range cases {
		if got := sanitizeNoteName(in); got != want {
			t.Errorf("sanitizeNoteName(%q) = %q, want %q", in, got, want)
		}
	}
}
