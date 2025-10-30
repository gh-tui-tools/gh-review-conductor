package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ItemRenderer defines how to render an item in the selector
type ItemRenderer[T any] interface {
	// Title returns the primary display text for an item
	Title(item T) string
	// Description returns secondary text for an item
	Description(item T) string
	// Preview returns detailed preview text for an item
	Preview(item T) string
	// EditPath returns the file path to open in editor (optional)
	EditPath(item T) string
	// EditLine returns the line number to go to in editor (1-based, optional)
	EditLine(item T) int
	// BrowserURL returns the URL to open in browser (optional)
	BrowserURL(item T) string
}

// SelectionModel is the tea.Model for interactive selection
type SelectionModel[T any] struct {
	list       list.Model
	items      []T
	result     []T
	windowSize tea.WindowSizeMsg
}

// Item wraps a generic item for the list model
type listItem[T any] struct {
	value T
	item  ItemRenderer[T]
}

func (i listItem[T]) FilterValue() string {
	return i.item.Title(i.value)
}

func (i listItem[T]) Title() string {
	return i.item.Title(i.value)
}

func (i listItem[T]) Description() string {
	return i.item.Description(i.value)
}

// SelectFromList creates an interactive selector for a list of items
// Returns selected items in order they were selected
func SelectFromList[T any](items []T, renderer ItemRenderer[T]) (T, error) {
	// Convert items to list items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = listItem[T]{value: item, item: renderer}
	}

	l := list.New(listItems, itemDelegate[T]{renderer}, 100, 20)
	l.Title = "Select an item"
	l.SetShowStatusBar(true)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)

	m := &SelectionModel[T]{
		list:   l,
		items:  items,
		result: make([]T, 0),
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return *new(T), err
	}

	m = finalModel.(*SelectionModel[T])
	if len(m.result) == 0 {
		return *new(T), fmt.Errorf("no selection made")
	}

	return m.result[0], nil
}

// Init initializes the model
func (m *SelectionModel[T]) Init() tea.Cmd {
	return nil
}

// Update handles user input
func (m *SelectionModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				m.result = append(m.result, item.value)
			}
			return m, tea.Quit
		case "ctrl+e":
			// Edit in $EDITOR
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				if editPath := item.item.EditPath(item.value); editPath != "" {
					_ = m.editInEditor(editPath, item.item.EditLine(item.value))
				}
			}
			return m, nil
		case "ctrl+b":
			// Open in browser
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				if browserURL := item.item.BrowserURL(item.value); browserURL != "" {
					_ = m.openInBrowser(browserURL)
				}
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.windowSize = msg
		h, v := lipgloss.NewStyle().GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// editInEditor opens a file in $EDITOR at the specified line
func (m *SelectionModel[T]) editInEditor(filePath string, lineNum int) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	var cmd *exec.Cmd
	if lineNum > 0 {
		// Use +line convention
		cmd = exec.Command(editor, fmt.Sprintf("+%d", lineNum), filePath)
	} else {
		cmd = exec.Command(editor, filePath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// openInBrowser opens a URL in the default browser (OS-agnostic)
func (m *SelectionModel[T]) openInBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS
		cmd = exec.Command("open", url)
	case "linux":
		// Linux
		cmd = exec.Command("xdg-open", url)
	case "windows":
		// Windows
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		// Fallback: try xdg-open
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Run()
}

// View renders the model
func (m *SelectionModel[T]) View() string {
	if m.list.SelectedItem() == nil {
		return ""
	}

	selected := m.list.SelectedItem().(listItem[T])
	preview := selected.item.Preview(selected.value)

	// Calculate preview width based on terminal width
	// Leave space for the list (roughly 40 chars) and some padding
	previewWidth := m.windowSize.Width - 45
	if previewWidth < 30 {
		previewWidth = 30
	}

	// Wrap preview text to the calculated width
	wrappedPreview := WrapText(preview, previewWidth)

	// Styling
	previewStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Foreground(lipgloss.Color("248")).
		Width(previewWidth)

	previewBox := previewStyle.Render(wrappedPreview)

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)
	help := helpStyle.Render("↑/↓ navigate  •  enter select  •  ctrl+e edit  •  ctrl+b browser  •  / search  •  q quit")

	// Top section with title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("36")).
		Bold(true)
	title := titleStyle.Render("Select a comment")

	// Combine: title + list
	listSection := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		m.list.View(),
	)

	// Side-by-side: list on left, preview on right
	mainContent := lipgloss.JoinHorizontal(
		lipgloss.Top,
		listSection,
		previewBox,
	)

	// Bottom: help text
	content := lipgloss.JoinVertical(
		lipgloss.Top,
		mainContent,
		help,
	)

	return content
}

// itemDelegate renders individual list items
type itemDelegate[T any] struct {
	renderer ItemRenderer[T]
}

func (d itemDelegate[T]) Height() int {
	return 1
}

func (d itemDelegate[T]) Spacing() int {
	return 0
}

func (d itemDelegate[T]) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d itemDelegate[T]) Render(w io.Writer, m list.Model, index int, item list.Item) {
	li := item.(listItem[T])
	title := d.renderer.Title(li.value)
	desc := d.renderer.Description(li.value)

	isSelected := index == m.Index()

	var s strings.Builder

	// Cursor
	cursor := "  "
	if isSelected {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("▶ ")
	}
	s.WriteString(cursor)

	// Title styling
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).
		Bold(isSelected)
	if !isSelected {
		titleStyle = titleStyle.Foreground(lipgloss.Color("250"))
	}
	s.WriteString(titleStyle.Render(title))

	// Description styling
	if desc != "" {
		s.WriteString(" ")
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		s.WriteString(descStyle.Render(desc))
	}

	_, _ = fmt.Fprint(w, s.String())
}
