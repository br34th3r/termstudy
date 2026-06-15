package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// deckItem adapts a Deck for the bubbles list, showing its due count.
type deckItem struct {
	deck *Deck
	due  int
}

func (d deckItem) Title() string { return d.deck.Name }
func (d deckItem) Description() string {
	if d.due == 0 {
		return fmt.Sprintf("%d cards · all reviewed", len(d.deck.Cards))
	}
	return fmt.Sprintf("%d cards · %d due", len(d.deck.Cards), d.due)
}
func (d deckItem) FilterValue() string { return d.deck.Name }

// decksLoadedMsg carries the result of scanning the decks directory.
type decksLoadedMsg struct {
	decks []*Deck
	err   error
}

// reloadDecksMsg requests a rescan (e.g. after a review session ends).
type reloadDecksMsg struct{}

// startReviewMsg asks the root model to begin reviewing a deck.
type startReviewMsg struct{ deck *Deck }

// decksModel lists decks and launches review sessions.
type decksModel struct {
	cfg     Config
	list    list.Model
	loadErr error
}

func newDecksModel(cfg Config) decksModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Decks — select to review due cards"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	return decksModel{cfg: cfg, list: l}
}

func (m decksModel) Init() tea.Cmd { return m.loadDecks() }

func (m decksModel) loadDecks() tea.Cmd {
	dir := m.cfg.DecksDir
	return func() tea.Msg {
		decks, err := LoadDecks(dir)
		return decksLoadedMsg{decks: decks, err: err}
	}
}

func (m decksModel) setSize(w, h int) decksModel {
	bodyH := h - 1
	if bodyH < 1 {
		bodyH = 1
	}
	m.list.SetSize(w, bodyH)
	return m
}

func (m decksModel) update(msg tea.Msg) (decksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case decksLoadedMsg:
		m.loadErr = msg.err
		today := truncateDay(time.Now())
		items := make([]list.Item, len(msg.decks))
		for i, d := range msg.decks {
			items[i] = deckItem{deck: d, due: d.DueCount(today)}
		}
		m.list.SetItems(items)
		return m, nil

	case reloadDecksMsg:
		return m, m.loadDecks()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, switchTo(screenMenu)
		case "r":
			return m, m.loadDecks()
		case "enter":
			if it, ok := m.list.SelectedItem().(deckItem); ok {
				deck := it.deck
				return m, func() tea.Msg { return startReviewMsg{deck: deck} }
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m decksModel) view() string {
	if m.loadErr != nil {
		return errLine(m.loadErr) + "\n" + helpBar("esc back")
	}
	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"No decks yet.\n\nAdd JSON decks to:\n  " + m.cfg.DecksDir +
				"\n\n(Ask Claude to generate a deck, then press 'r' to refresh.)")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("r refresh · esc back"))
	}
	help := helpBar("↑/↓ select · enter review · r refresh · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, m.list.View(), help)
}

func truncateDay(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}
