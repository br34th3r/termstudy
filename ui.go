package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// screen identifies the active top-level view.
type screen int

const (
	screenMenu screen = iota
	screenNotes
	screenDecks
	screenReview
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("213")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Padding(0, 1)
)

// switchMsg asks the root model to change screens.
type switchMsg struct{ to screen }

func switchTo(s screen) tea.Cmd {
	return func() tea.Msg { return switchMsg{to: s} }
}

// errMsg surfaces a recoverable error to the status line.
type errMsg struct{ err error }

// menuItem is one entry in the main menu list.
type menuItem struct {
	title, desc string
	target      screen
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string  { return m.desc }
func (m menuItem) FilterValue() string { return m.title }

// rootModel owns the sub-models and routes input to the active screen.
type rootModel struct {
	cfg    Config
	screen screen
	width  int
	height int
	err    error

	menu   list.Model
	notes  notesModel
	decks  decksModel
	review reviewModel
}

func newRootModel(cfg Config) rootModel {
	items := []list.Item{
		menuItem{"Notes", "Browse and edit your markdown notes", screenNotes},
		menuItem{"Review", "Study due flashcards with spaced repetition", screenDecks},
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "termstudy"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)

	return rootModel{
		cfg:    cfg,
		screen: screenMenu,
		menu:   l,
		notes:  newNotesModel(cfg),
		decks:  newDecksModel(cfg),
	}
}

func (m rootModel) Init() tea.Cmd {
	return tea.Batch(m.notes.Init(), m.decks.Init())
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		// Propagate size to every sub-model so switching is seamless.
		m.notes = m.notes.setSize(m.contentW(), m.contentH())
		m.decks = m.decks.setSize(m.contentW(), m.contentH())
		m.review = m.review.setSize(m.contentW(), m.contentH())
		return m, nil

	case switchMsg:
		m.screen = msg.to
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case startReviewMsg:
		m.review = newReviewModel(msg.deck).setSize(m.contentW(), m.contentH())
		m.screen = screenReview
		return m, m.review.Init()

	case reloadDecksMsg:
		// Refresh due counts when a review session ends.
		var cmd tea.Cmd
		m.decks, cmd = m.decks.update(msg)
		return m, cmd

	case tea.KeyMsg:
		// Global quit on the menu; sub-screens use Esc to go back.
		if m.screen == screenMenu && (msg.String() == "q" || msg.String() == "ctrl+c") {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	return m.routeToScreen(msg)
}

// routeToScreen delegates a message to the active sub-model.
func (m rootModel) routeToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenMenu:
		if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "enter") {
			if it, ok := m.menu.SelectedItem().(menuItem); ok {
				return m, switchTo(it.target)
			}
		}
		m.menu, cmd = m.menu.Update(msg)
	case screenNotes:
		m.notes, cmd = m.notes.update(msg)
	case screenDecks:
		m.decks, cmd = m.decks.update(msg)
	case screenReview:
		m.review, cmd = m.review.update(msg)
	}
	return m, cmd
}

func (m rootModel) View() string {
	var body string
	switch m.screen {
	case screenMenu:
		body = m.menu.View()
	case screenNotes:
		body = m.notes.view()
	case screenDecks:
		body = m.decks.view()
	case screenReview:
		body = m.review.view()
	}

	if m.err != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, body, errStyle.Render("error: "+m.err.Error()))
	}
	return body
}

// layout sizes the menu list to the current terminal.
func (m *rootModel) layout() {
	m.menu.SetSize(m.width, m.contentH())
}

func (m rootModel) contentW() int { return m.width }
func (m rootModel) contentH() int {
	h := m.height
	if h < 1 {
		return 1
	}
	return h
}

// helpBar renders a consistent footer hint line.
func helpBar(hints string) string {
	return statusStyle.Render(hints)
}

func errLine(err error) string {
	if err == nil {
		return ""
	}
	return errStyle.Render(fmt.Sprintf("error: %v", err))
}
