package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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

// noteCreatedMsg carries the result of creating a new note file.
type noteCreatedMsg struct {
	path string
	err  error
}

// notesModel is the split-pane note browser: a file list on the left and a
// rendered markdown preview on the right.
type notesModel struct {
	cfg      Config
	list     list.Model
	preview  viewport.Model
	input      textinput.Model
	creating   bool // entering a new note's title
	renaming   bool // editing the selected note's filename
	confirming bool // delete confirmation pending
	width      int
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

	ti := textinput.New()
	ti.Placeholder = "new note title (e.g. TCP handshake)"
	ti.CharLimit = 128

	return notesModel{cfg: cfg, list: l, input: ti}
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

func (m notesModel) createNote(title string) tea.Cmd {
	dir := m.cfg.NotesDir
	return func() tea.Msg {
		path, err := CreateNote(dir, title)
		return noteCreatedMsg{path: path, err: err}
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

	case noteCreatedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		// Open the new note in $EDITOR; editedMsg then reloads the list.
		return m, editPath(msg.path)

	case tea.KeyMsg:
		if m.creating || m.renaming {
			return m.updateInput(msg)
		}
		if m.confirming {
			return m.updateConfirmDelete(msg)
		}

		switch msg.String() {
		case "esc", "q":
			return m, switchTo(screenMenu)
		case "a":
			m.creating = true
			m.loadErr = nil
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case "R":
			if it, ok := m.list.SelectedItem().(noteItem); ok {
				m.renaming = true
				m.loadErr = nil
				m.input.SetValue(it.note.Title)
				m.input.CursorEnd()
				m.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "d":
			if _, ok := m.list.SelectedItem().(noteItem); ok {
				m.confirming = true
				m.loadErr = nil
			}
			return m, nil
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

// updateInput drives the shared text input used for creating and renaming.
func (m notesModel) updateInput(msg tea.KeyMsg) (notesModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating, m.renaming = false, false
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		creating := m.creating
		m.creating, m.renaming = false, false
		m.input.Blur()
		m.input.SetValue("")
		if val == "" {
			return m, nil
		}
		if creating {
			return m, m.createNote(val)
		}
		// Renaming the selected note.
		if it, ok := m.list.SelectedItem().(noteItem); ok {
			oldPath := it.note.Path
			return m, func() tea.Msg {
				if _, err := RenameNote(oldPath, val); err != nil {
					return errMsg{err}
				}
				notes, err := LoadNotes(m.cfg.NotesDir)
				return notesLoadedMsg{notes: notes, err: err}
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateConfirmDelete handles the y/n confirmation for deleting a note.
func (m notesModel) updateConfirmDelete(msg tea.KeyMsg) (notesModel, tea.Cmd) {
	m.confirming = false
	if msg.String() != "y" && msg.String() != "Y" {
		return m, nil
	}
	it, ok := m.list.SelectedItem().(noteItem)
	if !ok {
		return m, nil
	}
	path := it.note.Path
	// The deleted note may be the one shown in the preview; clear it.
	if path == m.curPath {
		m.curPath = ""
		m.preview.SetContent("")
	}
	return m, func() tea.Msg {
		if err := DeleteNote(path); err != nil {
			return errMsg{err}
		}
		notes, err := LoadNotes(m.cfg.NotesDir)
		return notesLoadedMsg{notes: notes, err: err}
	}
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
	return editPath(it.note.Path)
}

// editPath suspends the TUI and opens path in $EDITOR, emitting editedMsg when
// the editor exits.
func editPath(path string) tea.Cmd {
	editor, args := editorCommand()
	args = append(args, path)
	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editedMsg{path: path, err: err}
	})
}

func (m notesModel) view() string {
	if m.creating {
		prompt := lipgloss.NewStyle().Padding(1, 2).Render(
			"New note title:\n\n" + m.input.View() +
				"\n\nCreates a .md file in this field's notes folder and opens your $EDITOR.")
		return lipgloss.JoinVertical(lipgloss.Left, prompt, helpBar("enter create · esc cancel"))
	}
	if m.renaming {
		prompt := lipgloss.NewStyle().Padding(1, 2).Render(
			"Rename note:\n\n" + m.input.View() +
				"\n\nRenames the file (a .md extension is kept).")
		return lipgloss.JoinVertical(lipgloss.Left, prompt, helpBar("enter save · esc cancel"))
	}
	if m.loadErr != nil {
		return errLine(m.loadErr) + "\n" + helpBar("a add note · r refresh · esc back")
	}
	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"No markdown notes yet.\n\nPress 'a' to create one, or add .md files to:\n  " + m.cfg.NotesDir +
				"\n\n(You can also ask Claude to generate study notes here, then press 'r'.)")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("a add note · r refresh · esc back"))
	}

	left := m.list.View()
	right := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(lipgloss.Color("238")).
		PaddingLeft(1).
		Render(m.preview.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	help := helpBar("↑/↓ select · e edit · a add · R rename · d delete · J/K scroll · r refresh · esc back")
	if m.confirming {
		name := ""
		if it, ok := m.list.SelectedItem().(noteItem); ok {
			name = it.note.Title
		}
		help = helpBar(fmt.Sprintf("delete note %q? y confirm · any other key cancel", name))
	}
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
