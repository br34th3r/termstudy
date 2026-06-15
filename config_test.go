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
