package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	frontStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	backStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	progressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	doneStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("120")).Padding(1, 2)
)

// reviewModel drives a single study session over a deck's due cards. Cards
// graded "Again" are requeued to the back of the session queue.
type reviewModel struct {
	deck     *Deck
	queue    []*Card // remaining cards this session (front of slice == current)
	revealed bool
	width    int
	height   int
	vp       viewport.Model
	ready    bool

	reviewed int  // cards graded at least once
	again    int  // number of Again grades
	saveErr  error
}

func newReviewModel(deck *Deck) reviewModel {
	today := truncateDay(time.Now())
	return reviewModel{
		deck:  deck,
		queue: deck.DueCards(today),
	}
}

func (m reviewModel) Init() tea.Cmd { return nil }

func (m reviewModel) setSize(w, h int) reviewModel {
	m.width, m.height = w, h
	bodyH := h - 4
	if bodyH < 1 {
		bodyH = 1
	}
	if !m.ready {
		m.vp = viewport.New(w, bodyH)
		m.ready = true
	} else {
		m.vp.Width = w
		m.vp.Height = bodyH
	}
	m.refreshContent()
	return m
}

func (m reviewModel) current() *Card {
	if len(m.queue) == 0 {
		return nil
	}
	return m.queue[0]
}

func (m reviewModel) update(msg tea.Msg) (reviewModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			return m, endSession()
		case "ctrl+c":
			return m, tea.Quit
		}

		if m.current() == nil {
			// Session finished: any key returns to the deck list.
			return m, endSession()
		}

		if !m.revealed {
			switch key.String() {
			case " ", "enter":
				m.revealed = true
				m.refreshContent()
			}
			return m, nil
		}

		// Answer is revealed: accept a grade.
		switch key.String() {
		case "1":
			return m.grade(GradeAgain)
		case "2":
			return m.grade(GradeHard)
		case "3":
			return m.grade(GradeGood)
		case "4":
			return m.grade(GradeEasy)
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// grade applies an SM-2 grade to the current card, persists the deck, advances
// the queue, and requeues lapsed cards.
func (m reviewModel) grade(g Grade) (reviewModel, tea.Cmd) {
	card := m.current()
	if card == nil {
		return m, nil
	}
	card.Review(g, truncateDay(time.Now()))
	m.reviewed++
	if g == GradeAgain {
		m.again++
	}

	// Persist after every grade so progress is never lost.
	if err := m.deck.Save(); err != nil {
		m.saveErr = err
	}

	// Advance: pop current, requeue if it was a lapse.
	m.queue = m.queue[1:]
	if g == GradeAgain {
		m.queue = append(m.queue, card)
	}

	m.revealed = false
	m.refreshContent()
	return m, nil
}

// refreshContent rebuilds the viewport body for the current card/state.
func (m *reviewModel) refreshContent() {
	if !m.ready {
		return
	}
	card := m.current()
	if card == nil {
		m.vp.SetContent("")
		return
	}

	var b strings.Builder
	b.WriteString(frontStyle.Render(renderMD(card.Front, m.vp.Width)))
	if m.revealed {
		b.WriteString("\n")
		b.WriteString(dividerStyle.Render(strings.Repeat("─", min(m.vp.Width, 60))))
		b.WriteString("\n")
		b.WriteString(backStyle.Render(renderMD(card.Back, m.vp.Width)))
	}
	m.vp.SetContent(b.String())
	m.vp.GotoTop()
}

func (m reviewModel) view() string {
	if !m.ready {
		return ""
	}
	if m.current() == nil {
		summary := fmt.Sprintf("Session complete — reviewed %d card(s), %d needed another look.",
			m.reviewed, m.again)
		body := doneStyle.Render("✓ All caught up!\n\n" + summary)
		return lipgloss.JoinVertical(lipgloss.Left, body, helpBar("any key · esc back to decks"))
	}

	header := progressStyle.Render(fmt.Sprintf(" %s — %d left ", m.deck.Name, len(m.queue)))
	var help string
	if m.revealed {
		help = helpBar("grade:  1 again · 2 hard · 3 good · 4 easy    esc quit")
	} else {
		help = helpBar("space/enter reveal answer · esc quit")
	}

	parts := []string{header, m.vp.View(), help}
	if m.saveErr != nil {
		parts = append(parts, errLine(fmt.Errorf("save failed: %w", m.saveErr)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// endSession returns to the deck list and triggers a due-count refresh.
func endSession() tea.Cmd {
	return tea.Batch(
		switchTo(screenDecks),
		func() tea.Msg { return reloadDecksMsg{} },
	)
}

// renderMD renders short markdown (card faces may contain formatting). On any
// error it falls back to the raw text.
func renderMD(s string, width int) string {
	if width < 1 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(width))
	if err != nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return strings.TrimRight(out, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
