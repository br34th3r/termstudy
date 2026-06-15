package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fieldItem adapts a Field for the bubbles list, showing its note/deck counts.
type fieldItem struct{ field Field }

func (f fieldItem) Title() string { return f.field.Name }
func (f fieldItem) Description() string {
	return fmt.Sprintf("%d notes · %d decks", f.field.Notes, f.field.Decks)
}
func (f fieldItem) FilterValue() string { return f.field.Name }

// fieldsLoadedMsg carries the result of scanning for fields.
type fieldsLoadedMsg struct {
	fields []Field
	err    error
}

// fieldCreatedMsg reports the outcome of creating a new field.
type fieldCreatedMsg struct {
	field string
	err   error
}

// openFieldMsg asks the root model to enter a field, scoping notes and decks.
type openFieldMsg struct{ field string }

// fieldsModel is the entry screen: it lists study fields and lets the user
// create new ones. Picking a field scopes the rest of the app to it.
type fieldsModel struct {
	cfg        Config
	list       list.Model
	input      textinput.Model
	creating   bool
	renaming   bool   // editing the selected field's name
	renameFrom string // original name while renaming
	confirming bool   // delete confirmation pending
	loadErr    error
	width      int
	height     int
}

func newFieldsModel(cfg Config) fieldsModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Fields — pick a subject to study"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	ti := textinput.New()
	ti.Placeholder = "new field name (e.g. CyberSecurity)"
	ti.CharLimit = 64

	return fieldsModel{cfg: cfg, list: l, input: ti}
}

func (m fieldsModel) Init() tea.Cmd { return m.loadFields() }

func (m fieldsModel) loadFields() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		fields, err := LoadFields(cfg)
		return fieldsLoadedMsg{fields: fields, err: err}
	}
}

func (m fieldsModel) createField(name string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		field, err := cfg.CreateField(name)
		return fieldCreatedMsg{field: field, err: err}
	}
}

func (m fieldsModel) renameField(old, newName string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		name, err := cfg.RenameField(old, newName)
		return fieldCreatedMsg{field: name, err: err}
	}
}

func (m fieldsModel) deleteField(name string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		if err := cfg.DeleteField(name); err != nil {
			return errMsg{err}
		}
		fields, err := LoadFields(cfg)
		return fieldsLoadedMsg{fields: fields, err: err}
	}
}

func (m fieldsModel) setSize(w, h int) fieldsModel {
	m.width, m.height = w, h
	bodyH := h - 1 // reserve a help line
	if bodyH < 1 {
		bodyH = 1
	}
	m.list.SetSize(w, bodyH)
	return m
}

func (m fieldsModel) update(msg tea.Msg) (fieldsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case fieldsLoadedMsg:
		m.loadErr = msg.err
		items := make([]list.Item, len(msg.fields))
		for i, f := range msg.fields {
			items[i] = fieldItem{f}
		}
		m.list.SetItems(items)
		return m, nil

	case fieldCreatedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		// Reload so the new field shows up, then select it.
		return m, tea.Batch(m.loadFields(), selectField(msg.field))

	case selectFieldMsg:
		m.selectByName(msg.field)
		return m, nil

	case tea.KeyMsg:
		if m.creating || m.renaming {
			return m.updateInput(msg)
		}
		if m.confirming {
			return m.updateConfirmDelete(msg)
		}

		switch msg.String() {
		case "esc", "q":
			return m, tea.Quit
		case "n":
			m.creating = true
			m.loadErr = nil
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case "e":
			if it, ok := m.list.SelectedItem().(fieldItem); ok {
				m.renaming = true
				m.renameFrom = it.field.Name
				m.loadErr = nil
				m.input.SetValue(it.field.Name)
				m.input.CursorEnd()
				m.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "d":
			if _, ok := m.list.SelectedItem().(fieldItem); ok {
				m.confirming = true
				m.loadErr = nil
			}
			return m, nil
		case "r":
			return m, m.loadFields()
		case "enter":
			if it, ok := m.list.SelectedItem().(fieldItem); ok {
				name := it.field.Name
				return m, func() tea.Msg { return openFieldMsg{field: name} }
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// updateInput drives the shared text input used for creating and renaming.
func (m fieldsModel) updateInput(msg tea.KeyMsg) (fieldsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating, m.renaming = false, false
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case "enter":
		name := m.input.Value()
		creating := m.creating
		from := m.renameFrom
		m.creating, m.renaming = false, false
		m.input.Blur()
		m.input.SetValue("")
		if name == "" {
			return m, nil
		}
		if creating {
			return m, m.createField(name)
		}
		return m, m.renameField(from, name)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateConfirmDelete handles the y/n confirmation for deleting a field.
func (m fieldsModel) updateConfirmDelete(msg tea.KeyMsg) (fieldsModel, tea.Cmd) {
	m.confirming = false
	if msg.String() != "y" && msg.String() != "Y" {
		return m, nil
	}
	if it, ok := m.list.SelectedItem().(fieldItem); ok {
		return m, m.deleteField(it.field.Name)
	}
	return m, nil
}

// selectByName moves the cursor to the named field if it is present.
func (m *fieldsModel) selectByName(name string) {
	for i, it := range m.list.Items() {
		if fi, ok := it.(fieldItem); ok && fi.field.Name == name {
			m.list.Select(i)
			return
		}
	}
}

func (m fieldsModel) view() string {
	if m.creating {
		prompt := lipgloss.NewStyle().Padding(1, 2).Render(
			"New field name:\n\n" + m.input.View() +
				"\n\nCreates matching folders under notes/ and decks/.")
		return lipgloss.JoinVertical(lipgloss.Left, prompt, helpBar("enter create · esc cancel"))
	}
	if m.renaming {
		prompt := lipgloss.NewStyle().Padding(1, 2).Render(
			"Rename field:\n\n" + m.input.View() +
				"\n\nMoves its notes/ and decks/ folders to the new name.")
		return lipgloss.JoinVertical(lipgloss.Left, prompt, helpBar("enter save · esc cancel"))
	}
	if m.loadErr != nil {
		return errLine(m.loadErr) + "\n" + helpBar("n new field · r refresh · esc quit")
	}
	if len(m.list.Items()) == 0 {
		msg := lipgloss.NewStyle().Padding(1, 2).Render(
			"No fields yet.\n\nA field is a subject — e.g. CyberSecurity or Spanish —\n" +
				"with its own notes and flashcards.\n\nPress 'n' to create your first field.")
		return lipgloss.JoinVertical(lipgloss.Left, msg, helpBar("n new field · esc quit"))
	}
	help := helpBar("↑/↓ select · enter open · n new · e rename · d delete · r refresh · esc quit")
	if m.confirming {
		name := ""
		if it, ok := m.list.SelectedItem().(fieldItem); ok {
			name = it.field.Name
		}
		help = helpBar(fmt.Sprintf("delete field %q and ALL its notes & decks? y confirm · any other key cancel", name))
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.list.View(), help)
}

// selectFieldMsg asks the fields list to highlight a field by name.
type selectFieldMsg struct{ field string }

func selectField(name string) tea.Cmd {
	return func() tea.Msg { return selectFieldMsg{field: name} }
}
