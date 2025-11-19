package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
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
	// FilterValue returns the string to match against when filtering
	FilterValue(item T) string
}

// CustomAction is a function that handles custom actions on items
type CustomAction[T any] func(item T) (string, error)

// SelectionModel is the tea.Model for interactive selection
type SelectionModel[T any] struct {
	list         list.Model
	items        []T
	result       []T
	windowSize   tea.WindowSizeMsg
	customAction CustomAction[T]
	actionKey    string // Key binding description for custom action (e.g., "ctrl+r resolve")
	onOpen       CustomAction[T]
	viewport     viewport.Model
	showDetail   bool
	filterFunc   func(T, bool) bool
	filterActive bool
	renderer     ItemRenderer[T]
	showHelp     bool
	onSelect     CustomAction[T]
}

// Item wraps a generic item for the list model
type listItem[T any] struct {
	value T
	item  ItemRenderer[T]
}

func (i listItem[T]) FilterValue() string {
	return i.item.FilterValue(i.value)
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
	return SelectFromListWithAction(items, renderer, nil, "", nil, nil, nil)
}

// SelectFromListWithAction creates an interactive selector with a custom action
// The customAction is triggered by Ctrl+R, and actionKey describes the action in the help text
func SelectFromListWithAction[T any](items []T, renderer ItemRenderer[T], customAction CustomAction[T], actionKey string, onOpen CustomAction[T], filterFunc func(T, bool) bool, onSelect CustomAction[T]) (T, error) {
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
	l.SetShowHelp(false)

	m := &SelectionModel[T]{
		list:         l,
		items:        items,
		result:       make([]T, 0),
		customAction: customAction,
		actionKey:    actionKey,
		onOpen:       onOpen,
		viewport:     viewport.New(0, 0),
		filterFunc:   filterFunc,
		renderer:     renderer,
		onSelect:     onSelect,
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
	return tea.Batch(tea.EnterAltScreen)
}

// Update handles user input
func (m *SelectionModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle detail view navigation
	if m.showDetail {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "q", "backspace":
				m.showDetail = false
				return m, nil
			}
		case tea.WindowSizeMsg:
			m.windowSize = msg
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If we are filtering, let the list handle the input
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "h":
			if m.filterFunc != nil {
				m.filterActive = !m.filterActive
				m.updateVisibleItems()
				return m, nil
			}
		case "o":
			if m.onOpen != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					msg, err := m.onOpen(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					if msg != "" {
						return m, m.list.NewStatusMessage(msg)
					}
				}
			}
			return m, nil
		case "enter":
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				
				if m.onSelect != nil {
					msg, err := m.onSelect(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					
					if msg == "SHOW_DETAIL" {
						// Show detail view
						m.showDetail = true
						content := item.item.Preview(item.value)
						wrappedContent := WrapText(content, m.viewport.Width)
						m.viewport.SetContent(wrappedContent)
						m.viewport.GotoTop()
						return m, nil
					}
					
					// Assume it was a toggle or action that requires refresh
					m.updateVisibleItems()
					// Force update of the item in the list to reflect changes
					m.list.SetItem(m.list.Index(), item)
					
					if msg != "" {
						return m, m.list.NewStatusMessage(msg)
					}
					return m, nil
				}
				
				if m.onOpen != nil {
					// Browse mode: Show Detail
					m.showDetail = true
					content := item.item.Preview(item.value)
					// Wrap content to viewport width
					wrappedContent := WrapText(content, m.viewport.Width)
					m.viewport.SetContent(wrappedContent)
					// Reset viewport position
					m.viewport.GotoTop()
					return m, nil
				}
				// Apply mode: Select and Return
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
		case "ctrl+r":
			// Custom action (e.g., resolve)
			if m.customAction != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					msg, err := m.customAction(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					// Force update of the item in the list to reflect changes
					m.list.SetItem(m.list.Index(), item)
					
					if msg != "" {
						return m, m.list.NewStatusMessage(msg)
					}
				}
			}
			return m, nil
		case "right", "l":
			// Also allow right arrow or 'l' to enter detail view if in browse mode
			if m.onOpen != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					m.showDetail = true
					content := item.item.Preview(item.value)
					wrappedContent := WrapText(content, m.viewport.Width)
					m.viewport.SetContent(wrappedContent)
					m.viewport.GotoTop()
					return m, nil
				}
			}
		}
	case tea.WindowSizeMsg:
		m.windowSize = msg
		h, v := lipgloss.NewStyle().GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
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

// updateVisibleItems updates the list items based on the current filter state
func (m *SelectionModel[T]) updateVisibleItems() {
	var visible []list.Item
	for _, item := range m.items {
		if m.filterFunc == nil || m.filterFunc(item, m.filterActive) {
			visible = append(visible, listItem[T]{value: item, item: m.renderer})
		}
	}
	m.list.SetItems(visible)
}

// View renders the model
func (m *SelectionModel[T]) View() string {
	if m.showDetail {
		return m.viewport.View()
	}

	if m.list.SelectedItem() == nil {
		return ""
	}

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")). // Brighter gray for better visibility
		Italic(true)

	var helpText string
	if !m.showHelp {
		// Compact view
		helpText = "↑/↓ navigate  •  enter details  •  q quit  •  ? more"
	} else {
		// Expanded view
		helpText = "↑/↓ navigate  •  enter details  •  q quit  •  ? less\n"
		
		var extraCommands []string
		if m.onOpen != nil {
			extraCommands = append(extraCommands, "o open browser")
		}
		extraCommands = append(extraCommands, "ctrl+e edit")
		if m.filterFunc != nil {
			extraCommands = append(extraCommands, "h toggle resolved")
		}
		if m.customAction != nil && m.actionKey != "" {
			extraCommands = append(extraCommands, m.actionKey)
		}
		extraCommands = append(extraCommands, "/ search")
		
		helpText += strings.Join(extraCommands, "  •  ")
	}
	
	help := helpStyle.Render(helpText)

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

	// Bottom: help text
	content := lipgloss.JoinVertical(
		lipgloss.Top,
		listSection,
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

	if isSelected {
		// Cursor
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("▶ "))

		// Selected Item Style
		// We use a background color to make it prominent
		selectedBg := lipgloss.Color("57")  // Indigo/Purple
		selectedFg := lipgloss.Color("255") // White

		titleStyle := lipgloss.NewStyle().
			Foreground(selectedFg).
			Background(selectedBg).
			Bold(true)

		s.WriteString(titleStyle.Render(title))

		if desc != "" {
			descStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")). // Slightly dimmer than white
				Background(selectedBg)
			s.WriteString(descStyle.Render(" " + desc))
		}
	} else {
		// Unselected
		s.WriteString("  ")

		titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		s.WriteString(titleStyle.Render(title))

		if desc != "" {
			s.WriteString(" ")
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
			s.WriteString(descStyle.Render(desc))
		}
	}

	_, _ = fmt.Fprint(w, s.String())
}
