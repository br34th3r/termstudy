package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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

// deckCreatedMsg carries the result of creating a new deck.
type deckCreatedMsg struct {
	deck *Deck
	err  error
}

// deckMode is the decks screen's current sub-view.
type deckMode int

const (
	deckModeList deckMode = iota
	deckModeNewDeck
	deckModeRename
)

// decksModel lists decks and is the hub for managing them: launching review,
// creating, renaming, deleting, and opening a deck's card manager.
type decksModel struct {
	cfg     Config
	list    list.Model
	loadErr error

	mode       deckMode
	input      textinput.Model // new-deck name / rename
	renaming   *Deck           // deck being renamed (deckModeRename)
	confirming bool            // delete confirmation pending
	formErr    error
}

func newDecksModel(cfg Config) decksModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Decks — select to review due cards"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	in := textinput.New()
	in.Placeholder = "deck name (e.g. Irregular verbs)"
	in.CharLimit = 64

	return decksModel{cfg: cfg, list: l, input: in}
}

func (m decksModel) Init() tea.Cmd { return m.loadDecks() }

func (m decksModel) loadDecks() tea.Cmd {
	dir := m.cfg.DecksDir
	return func() tea.Msg {
		decks, err := LoadDecks(dir)
		return decksLoadedMsg{decks: decks, err: err}
	}
}

func (m decksModel) createDeck(name string) tea.Cmd {
	dir := m.cfg.DecksDir
	return func() tea.Msg {
		d, err := CreateDeck(dir, name)
		return deckCreatedMsg{deck: d, err: err}
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

	case deckCreatedMsg:
		if msg.err != nil {
			m.formErr = msg.err
			m.mode = deckModeList
			return m, nil
		}
		// New deck created — reload the list and open its card manager in add
		// mode so the user can start entering cards right away.
		m.mode = deckModeList
		deck := msg.deck
		return m, tea.Batch(
			m.loadDecks(),
			func() tea.Msg { return manageCardsMsg{deck: deck, startAdd: true} },
		)

	case tea.KeyMsg:
		switch m.mode {
		case deckModeNewDeck:
			return m.updateNewDeck(msg)
		case deckModeRename:
			return m.updateRename(msg)
		}

		if m.confirming {
			return m.updateConfirmDelete(msg)
		}

		switch msg.String() {
		case "esc", "q":
			return m, switchTo(screenMenu)
		case "r":
			return m, m.loadDecks()
		case "n":
			m.mode = deckModeNewDeck
			m.formErr = nil
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case "e":
			if it, ok := m.list.SelectedItem().(deckItem); ok {
				m.mode = deckModeRename
				m.renaming = it.deck
				m.formErr = nil
				m.input.SetValue(it.deck.Name)
				m.input.CursorEnd()
				m.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "d":
			if _, ok := m.list.SelectedItem().(deckItem); ok {
				m.confirming = true
				m.formErr = nil
			}
			return m, nil
		case "c":
			if it, ok := m.list.SelectedItem().(deckItem); ok {
				deck := it.deck
				return m, func() tea.Msg { return manageCardsMsg{deck: deck} }
			}
			return m, nil
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

// updateNewDeck handles keys while the new-deck name prompt is active.
func (m decksModel) updateNewDeck(msg tea.KeyMsg) (decksModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = deckModeList
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case "enter":
		name := m.input.Value()
		m.input.Blur()
		m.input.SetValue("")
		if strings.TrimSpace(name) == "" {
			m.mode = deckModeList
			return m, nil
		}
		return m, m.createDeck(name)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateRename handles keys while renaming the selected deck.
func (m decksModel) updateRename(msg tea.KeyMsg) (decksModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = deckModeList
		m.input.Blur()
		m.input.SetValue("")
		m.renaming = nil
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.input.Value())
		m.input.Blur()
		m.input.SetValue("")
		m.mode = deckModeList
		if name != "" && m.renaming != nil {
			if err := m.renaming.Rename(name); err != nil {
				m.formErr = err
			}
		}
		m.renaming = nil
		return m, m.loadDecks()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateConfirmDelete handles the y/n delete confirmation for the selected deck.
func (m decksModel) updateConfirmDelete(msg tea.KeyMsg) (decksModel, tea.Cmd) {
	m.confirming = false
	if msg.String() != "y" && msg.String() != "Y" {
		return m, nil
	}
	if it, ok := m.list.SelectedItem().(deckItem); ok {
		if err := it.deck.Delete(); err != nil {
			m.formErr = err
			return m, nil
		}
	}
	return m, m.loadDecks()
}

func (m decksModel) view() string {
	switch m.mode {
	case deckModeNewDeck:
		return m.promptView("New deck name:", "Creates an empty JSON deck; you can add cards next.", "enter create · esc cancel")
	case deckModeRename:
		return m.promptView("Rename deck:", "Updates the deck's display name.", "enter save · esc cancel")
	}

	if m.loadErr != nil {
		return errLine(m.loadErr) + "\n" + helpBar("n new deck · r refresh · esc back")
	}
	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"No decks yet.\n\nPress 'n' to create one, or add JSON decks to:\n  " + m.cfg.DecksDir +
				"\n\n(You can also ask Claude to generate a deck, then press 'r'.)")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("n new deck · r refresh · esc back"))
	}

	help := helpBar("↑/↓ select · enter review · c cards · n new · e rename · d delete · r refresh · esc back")
	if m.confirming {
		name := ""
		if it, ok := m.list.SelectedItem().(deckItem); ok {
			name = it.deck.Name
		}
		help = helpBar(fmt.Sprintf("delete deck %q and all its cards? y confirm · any other key cancel", name))
	}
	parts := []string{m.list.View()}
	if m.formErr != nil {
		parts = append(parts, errLine(m.formErr))
	}
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// promptView renders the shared single-input prompt used for new/rename.
func (m decksModel) promptView(title, hint, help string) string {
	prompt := lipgloss.NewStyle().Padding(1, 2).Render(
		title + "\n\n" + m.input.View() + "\n\n" + hint)
	parts := []string{prompt}
	if m.formErr != nil {
		parts = append(parts, errLine(m.formErr))
	}
	parts = append(parts, helpBar(help))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func truncateDay(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}
