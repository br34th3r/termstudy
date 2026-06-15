package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// manageCardsMsg asks the root model to open the card manager for a deck. When
// startAdd is set, the manager opens straight into the add-card form (used right
// after creating a new deck, so cards can be entered immediately).
type manageCardsMsg struct {
	deck     *Deck
	startAdd bool
}

// cardItem adapts a Card for the bubbles list, previewing its faces.
type cardItem struct{ card *Card }

func (c cardItem) Title() string       { return oneLine(c.card.Front, 80) }
func (c cardItem) Description() string { return oneLine(c.card.Back, 80) }
func (c cardItem) FilterValue() string { return c.card.Front }

// cardMode is the card manager's current sub-view.
type cardMode int

const (
	cardModeList cardMode = iota
	cardModeForm // add or edit, distinguished by editing != nil
)

// cardsModel manages the cards of a single deck: listing, adding, editing, and
// deleting them. Changes are persisted to the deck file immediately.
type cardsModel struct {
	deck    *Deck
	list    list.Model
	mode    cardMode
	front   textinput.Model
	back    textinput.Model
	focus   int   // active form field: 0=front, 1=back
	editing *Card // card being edited; nil while adding
	added   int   // cards added in the current add session

	confirming bool // delete confirmation pending
	formErr    error
	width      int
	height     int
}

func newCardsModel(deck *Deck) cardsModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Cards · " + deck.Name
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	front := textinput.New()
	front.Placeholder = "front (question / prompt)"
	front.CharLimit = 512

	back := textinput.New()
	back.Placeholder = "back (answer)"
	back.CharLimit = 1024

	m := cardsModel{deck: deck, list: l, front: front, back: back}
	m.refreshItems()
	return m
}

func (m cardsModel) Init() tea.Cmd { return nil }

func (m cardsModel) setSize(w, h int) cardsModel {
	m.width, m.height = w, h
	if m.deck == nil {
		return m // zero-value placeholder before a deck is opened
	}
	bodyH := h - 1
	if bodyH < 1 {
		bodyH = 1
	}
	m.list.SetSize(w, bodyH)
	return m
}

// refreshItems rebuilds the list from the deck's current cards.
func (m *cardsModel) refreshItems() {
	items := make([]list.Item, len(m.deck.Cards))
	for i, c := range m.deck.Cards {
		items[i] = cardItem{c}
	}
	m.list.SetItems(items)
}

func (m cardsModel) update(msg tea.Msg) (cardsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	if m.mode == cardModeForm {
		return m.updateForm(key)
	}

	if m.confirming {
		switch key.String() {
		case "y", "Y":
			m.confirming = false
			if it, ok := m.list.SelectedItem().(cardItem); ok {
				if err := m.deck.RemoveCard(it.card); err != nil {
					m.formErr = err
				} else {
					m.refreshItems()
				}
			}
			return m, nil
		default:
			m.confirming = false
			return m, nil
		}
	}

	switch key.String() {
	case "esc", "q":
		return m, leaveCards()
	case "a":
		m.startAdd()
		return m, textinput.Blink
	case "e", "enter":
		if it, ok := m.list.SelectedItem().(cardItem); ok {
			m.startEdit(it.card)
			return m, textinput.Blink
		}
		return m, nil
	case "d":
		if _, ok := m.list.SelectedItem().(cardItem); ok {
			m.confirming = true
			m.formErr = nil
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(key)
	return m, cmd
}

// startAdd opens the form for a brand-new card.
func (m *cardsModel) startAdd() {
	m.mode = cardModeForm
	m.editing = nil
	m.formErr = nil
	m.front.SetValue("")
	m.back.SetValue("")
	m.setFocus(0)
}

// startEdit opens the form pre-filled with an existing card's faces.
func (m *cardsModel) startEdit(c *Card) {
	m.mode = cardModeForm
	m.editing = c
	m.formErr = nil
	m.front.SetValue(c.Front)
	m.back.SetValue(c.Back)
	m.setFocus(0)
}

func (m *cardsModel) setFocus(i int) {
	m.focus = i
	if i == 0 {
		m.front.Focus()
		m.back.Blur()
	} else {
		m.back.Focus()
		m.front.Blur()
	}
}

// updateForm handles keys while the add/edit form is active.
func (m cardsModel) updateForm(key tea.KeyMsg) (cardsModel, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode = cardModeList
		m.front.Blur()
		m.back.Blur()
		m.formErr = nil
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.setFocus(1 - m.focus)
		return m, textinput.Blink
	case "enter":
		front := strings.TrimSpace(m.front.Value())
		back := strings.TrimSpace(m.back.Value())
		if front == "" || back == "" {
			m.formErr = fmt.Errorf("both front and back are required")
			if front == "" {
				m.setFocus(0)
			} else {
				m.setFocus(1)
			}
			return m, textinput.Blink
		}
		if m.editing != nil {
			// Editing: save the change and return to the list.
			if err := m.deck.UpdateCard(m.editing, front, back); err != nil {
				m.formErr = err
				return m, nil
			}
			m.refreshItems()
			m.mode = cardModeList
			m.front.Blur()
			m.back.Blur()
			return m, nil
		}
		// Adding: append and keep the form open for the next card.
		if err := m.deck.AddCard(front, back); err != nil {
			m.formErr = err
			return m, nil
		}
		m.added++
		m.formErr = nil
		m.refreshItems()
		m.front.SetValue("")
		m.back.SetValue("")
		m.setFocus(0)
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	if m.focus == 0 {
		m.front, cmd = m.front.Update(key)
	} else {
		m.back, cmd = m.back.Update(key)
	}
	return m, cmd
}

func (m cardsModel) view() string {
	if m.mode == cardModeForm {
		return m.formView()
	}

	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"This deck has no cards yet.\n\nPress 'a' to add your first card.")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("a add card · esc back"))
	}

	help := helpBar("↑/↓ select · a add · e/enter edit · d delete · esc back")
	if m.confirming {
		help = helpBar("delete this card? y confirm · any other key cancel")
	}
	parts := []string{m.list.View()}
	if m.formErr != nil {
		parts = append(parts, errLine(m.formErr))
	}
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// formView renders the add/edit card form.
func (m cardsModel) formView() string {
	verb := "Add card"
	if m.editing != nil {
		verb = "Edit card"
	}
	header := titleStyle.Render(verb + " · " + m.deck.Name)
	form := lipgloss.NewStyle().Padding(1, 2).Render(
		"Front:\n" + m.front.View() + "\n\nBack:\n" + m.back.View())

	parts := []string{header, form}
	if m.editing == nil && m.added > 0 {
		parts = append(parts, statusStyle.Render(fmt.Sprintf("added %d card(s)", m.added)))
	}
	if m.formErr != nil {
		parts = append(parts, errLine(m.formErr))
	}
	saveHint := "enter save & next"
	if m.editing != nil {
		saveHint = "enter save"
	}
	parts = append(parts, helpBar("tab switch field · "+saveHint+" · esc "+doneOrCancel(m.editing)))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func doneOrCancel(editing *Card) string {
	if editing != nil {
		return "cancel"
	}
	return "done"
}

// leaveCards returns to the deck list and refreshes its due/card counts.
func leaveCards() tea.Cmd {
	return tea.Batch(
		switchTo(screenDecks),
		func() tea.Msg { return reloadDecksMsg{} },
	)
}

// oneLine collapses whitespace/newlines and truncates s for list display.
func oneLine(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
