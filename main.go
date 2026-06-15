package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var (
		notesDir = flag.String("notes", "", "directory of markdown notes (default ~/.termstudy/notes)")
		decksDir = flag.String("decks", "", "directory of JSON flashcard decks (default ~/.termstudy/decks)")
		showPath = flag.Bool("paths", false, "print resolved data directories and exit")
	)
	flag.Parse()

	cfg := DefaultConfig(*notesDir, *decksDir)
	if err := cfg.Ensure(); err != nil {
		fmt.Fprintf(os.Stderr, "termstudy: %v\n", err)
		os.Exit(1)
	}

	if *showPath {
		fmt.Printf("notes: %s\ndecks: %s\n", cfg.NotesDir, cfg.DecksDir)
		fields, err := LoadFields(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "termstudy: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("fields:\n")
		for _, f := range fields {
			fmt.Printf("  %s (%d notes, %d decks)\n", f.Name, f.Notes, f.Decks)
		}
		return
	}

	p := tea.NewProgram(newRootModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "termstudy: %v\n", err)
		os.Exit(1)
	}
}
