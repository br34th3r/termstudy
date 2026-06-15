package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// noteItem adapts a Note for the bubbles list.
type noteItem struct{ note Note }

func (n noteItem) Title() string       { return n.note.Title }
func (n noteItem) Description() string  { return n.note.Path }
func (n noteItem) FilterValue() string { return n.note.Title }

// notesLoadedMsg carries the result of (re)scanning the notes directory.
type notesLoadedMsg struct {
	notes []Note
	err   error
}

// noteRenderedMsg carries glamour-rendered markdown for the preview pane.
type noteRenderedMsg struct {
	path    string
	content string
	err     error
}

// editedMsg fires after the external editor exits.
type editedMsg struct {
	path string
	err  error
}

// notesModel is the split-pane note browser: a file list on the left and a
// rendered markdown preview on the right.
type notesModel struct {
	cfg      Config
	list     list.Model
	preview  viewport.Model
	width    int
	height   int
	ready    bool
	curPath  string // path currently shown in preview
	loadErr  error
}

func newNotesModel(cfg Config) notesModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Notes"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	return notesModel{cfg: cfg, list: l}
}

func (m notesModel) Init() tea.Cmd {
	return m.loadNotes()
}

func (m notesModel) loadNotes() tea.Cmd {
	dir := m.cfg.NotesDir
	return func() tea.Msg {
		notes, err := LoadNotes(dir)
		return notesLoadedMsg{notes: notes, err: err}
	}
}

func (m notesModel) listWidth() int {
	w := m.width / 3
	if w < 24 {
		w = 24
	}
	if w > 48 {
		w = 48
	}
	return w
}

func (m notesModel) setSize(w, h int) notesModel {
	m.width, m.height = w, h
	bodyH := h - 1 // reserve a help line
	if bodyH < 1 {
		bodyH = 1
	}
	lw := m.listWidth()
	m.list.SetSize(lw, bodyH)
	pw := w - lw - 1
	if pw < 1 {
		pw = 1
	}
	if !m.ready {
		m.preview = viewport.New(pw, bodyH)
		m.ready = true
	} else {
		m.preview.Width = pw
		m.preview.Height = bodyH
	}
	return m
}

func (m notesModel) update(msg tea.Msg) (notesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case notesLoadedMsg:
		m.loadErr = msg.err
		items := make([]list.Item, len(msg.notes))
		for i, n := range msg.notes {
			items[i] = noteItem{n}
		}
		m.list.SetItems(items)
		return m, m.renderSelected()

	case noteRenderedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		m.curPath = msg.path
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		return m, nil

	case editedMsg:
		if msg.err != nil {
			return m, func() tea.Msg { return errMsg{msg.err} }
		}
		// Reload list (titles may change) and re-render the edited note.
		return m, m.loadNotes()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, switchTo(screenMenu)
		case "e", "enter":
			return m, m.editSelected()
		case "r":
			return m, m.loadNotes()
		case "j", "down", "k", "up":
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, tea.Batch(cmd, m.renderSelected())
		case "J":
			m.preview, _ = m.preview.Update(tea.KeyMsg{Type: tea.KeyDown})
			return m, nil
		case "K":
			m.preview, _ = m.preview.Update(tea.KeyMsg{Type: tea.KeyUp})
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// renderSelected renders the highlighted note's markdown if it isn't already shown.
func (m notesModel) renderSelected() tea.Cmd {
	it, ok := m.list.SelectedItem().(noteItem)
	if !ok {
		return nil
	}
	if it.note.Path == m.curPath {
		return nil
	}
	path := it.note.Path
	width := m.preview.Width
	return func() tea.Msg {
		raw, err := os.ReadFile(path)
		if err != nil {
			return noteRenderedMsg{path: path, err: err}
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return noteRenderedMsg{path: path, content: string(raw)}
		}
		out, err := r.Render(string(raw))
		if err != nil {
			return noteRenderedMsg{path: path, content: string(raw)}
		}
		return noteRenderedMsg{path: path, content: out}
	}
}

// editSelected suspends the TUI and opens the highlighted note in $EDITOR.
func (m notesModel) editSelected() tea.Cmd {
	it, ok := m.list.SelectedItem().(noteItem)
	if !ok {
		return nil
	}
	path := it.note.Path
	editor, args := editorCommand()
	args = append(args, path)
	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editedMsg{path: path, err: err}
	})
}

func (m notesModel) view() string {
	if m.loadErr != nil {
		return errLine(m.loadErr) + "\n" + helpBar("esc back")
	}
	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"No markdown notes yet.\n\nAdd .md files to:\n  " + m.cfg.NotesDir +
				"\n\n(Ask Claude to generate study notes here, then press 'r' to refresh.)")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("r refresh · esc back"))
	}

	left := m.list.View()
	right := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(lipgloss.Color("238")).
		PaddingLeft(1).
		Render(m.preview.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	help := helpBar("↑/↓ select · J/K scroll preview · e/enter edit in $EDITOR · r refresh · esc back")
	return lipgloss.JoinVertical(lipgloss.Left, body, help)
}

// editorCommand resolves the user's editor, falling back to vim then vi.
func editorCommand() (string, []string) {
	if e := strings.TrimSpace(os.Getenv("VISUAL")); e != "" {
		return splitEditor(e)
	}
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return splitEditor(e)
	}
	if _, err := exec.LookPath("vim"); err == nil {
		return "vim", nil
	}
	return "vi", nil
}

// splitEditor handles editors specified with flags, e.g. "code -w".
func splitEditor(e string) (string, []string) {
	parts := strings.Fields(e)
	if len(parts) == 0 {
		return "vi", nil
	}
	return parts[0], parts[1:]
}
