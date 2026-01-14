package ui

import (
	"errors"
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
	// IsSkippable returns true if the item should be skipped during navigation
	IsSkippable(item T) bool
}

// CustomAction is a function that handles custom actions on items
type CustomAction[T any] func(item T) (string, error)

// EditorPreparer returns the initial content for the editor, or error to abort
type EditorPreparer[T any] func(item T) (string, error)

// EditorCompleter is called with the editor content to complete the action
type EditorCompleter[T any] func(item T, editorContent string) (string, error)

// editorFinishedMsg is sent when an editor process completes
type editorFinishedMsg struct {
	err error
}

// loadDetailMsg triggers the actual detail loading after showing loading state
type loadDetailMsg struct{}

// refreshFinishedMsg signals that refresh has completed
type refreshFinishedMsg struct {
	items any // will be []T
	err   error
}

// agentFinishedMsg is sent when the coding agent process completes
type agentFinishedMsg struct {
	err error
}

var ErrNoSelection = errors.New("no selection made")

// SelectorOptions configures the interactive selector.
// Use this struct to configure all selector behavior in a readable way.
type SelectorOptions[T any] struct {
	// Required
	Items    []T
	Renderer ItemRenderer[T]

	// Core callbacks
	OnSelect       CustomAction[T]        // Called when Enter is pressed
	OnOpen         CustomAction[T]        // Called when 'o' is pressed
	FilterFunc     func(T, bool) bool     // Filter items based on state
	IsItemResolved func(T) bool           // For dynamic key display (r vs u)
	RefreshItems   func() ([]T, error)    // Called when 'i' is pressed

	// Action: r/u (resolve toggle)
	ResolveAction CustomAction[T]
	ResolveKey    string // e.g., "r resolve"
	ResolveKeyAlt string // e.g., "u unresolve"

	// Action: R/U (resolve+comment via editor)
	ResolveCommentPrepare  EditorPreparer[T]
	ResolveCommentComplete EditorCompleter[T]
	ResolveCommentKey      string // e.g., "R resolve+comment"
	ResolveCommentKeyAlt   string // e.g., "U unresolve+comment"

	// Action: Q (quote reply via editor)
	QuotePrepare  EditorPreparer[T]
	QuoteComplete EditorCompleter[T]
	QuoteKey      string // e.g., "Q quote"

	// Action: C (quote+context via editor)
	QuoteContextPrepare  EditorPreparer[T]
	QuoteContextComplete EditorCompleter[T]
	QuoteContextKey      string // e.g., "C quote+context"

	// Action: a (launch agent)
	AgentAction CustomAction[T]
	AgentKey    string // e.g., "a agent"

	// Action: e (edit file)
	EditAction CustomAction[T]
	EditKey    string // e.g., "e edit"
}

// SelectionModel is the tea.Model for interactive selection
type SelectionModel[T any] struct {
	list       list.Model
	items      []T
	result     []T
	windowSize tea.WindowSizeMsg
	viewport   viewport.Model
	showDetail bool
	showHelp   bool

	// Configuration (from SelectorOptions)
	opts         SelectorOptions[T]
	filterActive bool

	// Runtime state for refresh
	refreshing bool

	// State for pending editor operation
	pendingEditorItem    T
	pendingEditorTmpFile string
	pendingEditorAction  int // 2 = R/U, 3 = Q, 4 = C

	// Confirmation message that persists until user dismisses it
	confirmationMessage string

	// Loading state for detail view
	loadingDetail bool
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

// SelectFromList creates an interactive selector for a list of items.
// For more options, use Select() with SelectorOptions.
func SelectFromList[T any](items []T, renderer ItemRenderer[T]) (T, error) {
	return Select(SelectorOptions[T]{
		Items:    items,
		Renderer: renderer,
	})
}

// SelectFromListWithAction creates an interactive selector with a custom action.
// Deprecated: Use Select() with SelectorOptions for new code.
func SelectFromListWithAction[T any](items []T, renderer ItemRenderer[T], customAction CustomAction[T], actionKey string, onOpen CustomAction[T], filterFunc func(T, bool) bool, onSelect CustomAction[T], customActionSecond CustomAction[T], actionKeySecond string) (T, error) {
	return Select(SelectorOptions[T]{
		Items:         items,
		Renderer:      renderer,
		OnSelect:      onSelect,
		OnOpen:        onOpen,
		FilterFunc:    filterFunc,
		ResolveAction: customAction,
		ResolveKey:    actionKey,
		// Note: old API used customActionSecond for R key but it was a sync action.
		// The new API uses editor callbacks for R. For backward compat, we don't
		// support the old sync R action through this wrapper.
		ResolveCommentKey: actionKeySecond,
	})
}

// Select creates an interactive selector with the given options.
// This is the primary API for creating selectors.
func Select[T any](opts SelectorOptions[T]) (T, error) {
	// Convert items to list items
	listItems := make([]list.Item, len(opts.Items))
	for i, item := range opts.Items {
		listItems[i] = listItem[T]{value: item, item: opts.Renderer}
	}

	l := list.New(listItems, itemDelegate[T]{opts.Renderer}, 100, 20)

	colorOn := ColorsEnabled()

	var styles list.Styles
	if colorOn {
		// Brighten the status bar when filtering so the "N filtered" text remains readable.
		styles = list.DefaultStyles()
		styles.StatusBarFilterCount = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")). // Light gray for dark backgrounds
			Bold(true)
		styles.StatusBarActiveFilter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")). // Brighter for the active filter value
			Bold(true)
		styles.StatusBar = styles.StatusBar.Foreground(lipgloss.Color("247"))
	} else {
		styles = list.Styles{}
	}
	l.Styles = styles

	// Make the filter bar high-contrast so typed text is visible.
	l.FilterInput.Prompt = "Filter: "
	if colorOn {
		l.FilterInput.PromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")). // yellow-leaning white
			Bold(true)
		l.FilterInput.TextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")) // white text while typing
		l.FilterInput.Cursor.Style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("213")). // bright magenta cursor
			Bold(true)
	}
	l.SetShowFilter(true)

	l.Title = "Select an item"
	l.SetShowStatusBar(true)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	m := &SelectionModel[T]{
		list:     l,
		items:    opts.Items,
		result:   make([]T, 0),
		viewport: viewport.New(0, 0),
		opts:     opts,
	}

	if opts.FilterFunc != nil {
		m.filterActive = true
		m.updateVisibleItems()
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return *new(T), err
	}

	m = finalModel.(*SelectionModel[T])
	if len(m.result) == 0 {
		return *new(T), ErrNoSelection
	}

	return m.result[0], nil
}

// Init initializes the model
func (m *SelectionModel[T]) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen)
}

// Update handles user input
func (m *SelectionModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle editor completion
	if finished, ok := msg.(editorFinishedMsg); ok {
		return m.handleEditorFinished(finished)
	}

	// Handle agent completion
	if finished, ok := msg.(agentFinishedMsg); ok {
		if finished.err != nil {
			return m, m.list.NewStatusMessage(Colorize(ColorRed, "agent error: "+finished.err.Error()))
		}
		return m, nil
	}

	// Handle deferred detail loading
	if _, ok := msg.(loadDetailMsg); ok {
		m.loadingDetail = false
		selected := m.list.SelectedItem()
		if selected != nil {
			item := selected.(listItem[T])

			if m.opts.OnSelect != nil {
				result, err := m.opts.OnSelect(item.value)
				if err != nil {
					return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
				}

				if result == "SHOW_DETAIL" || strings.HasPrefix(result, "SHOW_DETAIL:") {
					m.showDetail = true
					// Reserve 1 line for footer (nav hint)
					m.viewport.Height = m.windowSize.Height - 1
					content := item.item.Preview(item.value)
					wrappedContent := WrapText(content, m.viewport.Width)
					m.viewport.SetContent(wrappedContent)
					m.viewport.GotoTop()
					// Check for warning message after SHOW_DETAIL:
					if strings.HasPrefix(result, "SHOW_DETAIL:") {
						warning := strings.TrimPrefix(result, "SHOW_DETAIL:")
						return m, m.list.NewStatusMessage(Colorize(ColorYellow, warning))
					}
					return m, nil
				}

				// Assume it was a toggle or action that requires refresh
				m.updateVisibleItems()
				m.list.SetItem(m.list.Index(), item)
				if result != "" {
					return m, m.list.NewStatusMessage(result)
				}
			}
		}
		return m, nil
	}

	// Handle refresh completion
	if refreshMsg, ok := msg.(refreshFinishedMsg); ok {
		m.refreshing = false
		if refreshMsg.err != nil {
			return m, m.list.NewStatusMessage(Colorize(ColorRed, fmt.Sprintf("refresh failed: %v", refreshMsg.err)))
		}
		// Type assert the items
		if newItems, ok := refreshMsg.items.([]T); ok {
			m.items = newItems
			m.updateVisibleItems()
			return m, m.list.NewStatusMessage(Colorize(ColorGreen, fmt.Sprintf("Refreshed: %d items", len(newItems))))
		}
		return m, nil
	}

	// Handle confirmation message dismissal - any key dismisses it
	if m.confirmationMessage != "" {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.confirmationMessage = ""
			return m, nil
		}
		// Handle window resize while showing confirmation
		if wsm, ok := msg.(tea.WindowSizeMsg); ok {
			m.windowSize = wsm
		}
		return m, nil
	}

	// Handle detail view navigation
	if m.showDetail {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "backspace", "left", "h", "q":
				m.showDetail = false
				return m, nil
			case "ctrl+f":
				// Page down in detail view
				m.viewport.ViewDown()
				return m, nil
			case "ctrl+b":
				// Page up in detail view
				m.viewport.ViewUp()
				return m, nil
			case "r", "u":
				// Execute resolve action from detail view (r=resolve, u=unresolve - both toggle)
				if m.opts.ResolveAction != nil {
					selected := m.list.SelectedItem()
					if selected != nil {
						item := selected.(listItem[T])
						statusMsg, err := m.opts.ResolveAction(item.value)
						m.showDetail = false
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						m.list.SetItem(m.list.Index(), item)
						if statusMsg != "" {
							return m, m.list.NewStatusMessage(statusMsg)
						}
					}
				}
				return m, nil
			case "R", "U":
				// Execute resolve+comment action from detail view
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					m.showDetail = false
					// Use editor action if available
					if m.opts.ResolveCommentPrepare != nil {
						initialContent, err := m.opts.ResolveCommentPrepare(item.value)
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						return m, m.startEditorForAction(item.value, 2, initialContent)
					}
				}
				return m, nil
			case "Q":
				// Execute quote action from detail view
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					m.showDetail = false
					// Use editor action if available
					if m.opts.QuotePrepare != nil {
						initialContent, err := m.opts.QuotePrepare(item.value)
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						return m, m.startEditorForAction(item.value, 3, initialContent)
					}
				}
				return m, nil
			case "C":
				// Execute quote+context action from detail view
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					m.showDetail = false
					// Use editor action if available
					if m.opts.QuoteContextPrepare != nil {
						initialContent, err := m.opts.QuoteContextPrepare(item.value)
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						return m, m.startEditorForAction(item.value, 4, initialContent)
					}
				}
				return m, nil
			case "a":
				// Launch coding agent from detail view
				if m.opts.AgentAction != nil {
					selected := m.list.SelectedItem()
					if selected != nil {
						item := selected.(listItem[T])
						m.showDetail = false
						result, err := m.opts.AgentAction(item.value)
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						if strings.HasPrefix(result, "LAUNCH_AGENT:") {
							prompt := strings.TrimPrefix(result, "LAUNCH_AGENT:")
							return m, m.launchAgent(prompt)
						}
						if result != "" {
							return m, m.list.NewStatusMessage(result)
						}
					}
				}
				return m, nil
			case "o":
				// Open in browser from detail view
				if m.opts.OnOpen != nil {
					selected := m.list.SelectedItem()
					if selected != nil {
						item := selected.(listItem[T])
						statusMsg, err := m.opts.OnOpen(item.value)
						if err != nil {
							return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
						}
						if statusMsg != "" {
							return m, m.list.NewStatusMessage(statusMsg)
						}
					}
				}
				return m, nil
			}
		case tea.WindowSizeMsg:
			m.windowSize = msg
			m.viewport.Width = msg.Width
			// Reserve 1 line for footer (nav hint)
			m.viewport.Height = msg.Height - 1
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	// When the help overlay is open, keep interaction scoped to the help view.
	if m.showHelp {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "?", "esc":
				m.showHelp = false
				return m, nil
			case "ctrl+c", "q":
				return m, tea.Quit
			}
		case tea.WindowSizeMsg:
			m.windowSize = msg
		}
		return m, nil
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
			if m.opts.FilterFunc != nil {
				m.filterActive = !m.filterActive
				m.updateVisibleItems()
				return m, nil
			}
		case "i":
			// Refresh items from data source
			if m.opts.RefreshItems != nil && !m.refreshing {
				m.refreshing = true
				return m, func() tea.Msg {
					items, err := m.opts.RefreshItems()
					return refreshFinishedMsg{items: items, err: err}
				}
			}
			return m, nil
		case "o":
			if m.opts.OnOpen != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					msg, err := m.opts.OnOpen(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					if msg != "" {
						return m, m.list.NewStatusMessage(msg)
					}
				}
			}
			return m, nil
		case "down", "j":
			m.list.CursorDown()
			// Skip items that are marked as skippable (e.g. preview lines)
			for {
				selectedItem := m.list.SelectedItem()
				if selectedItem == nil {
					break
				}
				item := selectedItem.(listItem[T])
				if !m.opts.Renderer.IsSkippable(item.value) {
					break
				}
				// If we hit the bottom and it's skippable, we can't go further down.
				// We should probably go back up to the last non-skippable item.
				if m.list.Index() == len(m.list.Items())-1 {
					m.list.CursorUp()
					continue // Loop will check if new item is skippable (it shouldn't be if we came from there)
				}
				m.list.CursorDown()
			}
			return m, nil
		case "up", "k":
			m.list.CursorUp()
			// Skip items that are marked as skippable
			for {
				selectedItem := m.list.SelectedItem()
				if selectedItem == nil {
					break
				}
				item := selectedItem.(listItem[T])
				if !m.opts.Renderer.IsSkippable(item.value) {
					break
				}
				// If we hit the top and it's skippable
				if m.list.Index() == 0 {
					m.list.CursorDown()
					continue
				}
				m.list.CursorUp()
			}
			return m, nil
		case "enter":
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])

				if m.opts.OnSelect != nil {
					// Show loading state and defer the actual work
					m.loadingDetail = true
					return m, func() tea.Msg { return loadDetailMsg{} }
				}

				if m.opts.OnOpen != nil {
					// Browse mode: Show Detail (no deferred loading needed here)
					m.showDetail = true
					// Reserve 1 line for footer (nav hint)
					m.viewport.Height = m.windowSize.Height - 1
					content := item.item.Preview(item.value)
					wrappedContent := WrapText(content, m.viewport.Width)
					m.viewport.SetContent(wrappedContent)
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
					return m, m.editInEditor(editPath, item.item.EditLine(item.value))
				}
			}
			return m, nil
		case "r", "u":
			// Resolve/unresolve action (both toggle)
			if m.opts.ResolveAction != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					msg, err := m.opts.ResolveAction(item.value)
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
		case "R", "U":
			// Resolve+comment action via editor
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				// Use editor action if available
				if m.opts.ResolveCommentPrepare != nil {
					initialContent, err := m.opts.ResolveCommentPrepare(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					return m, m.startEditorForAction(item.value, 2, initialContent)
				}
			}
			return m, nil
		case "Q":
			// Quote reply action via editor
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				// Use editor action if available
				if m.opts.QuotePrepare != nil {
					initialContent, err := m.opts.QuotePrepare(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					return m, m.startEditorForAction(item.value, 3, initialContent)
				}
			}
			return m, nil
		case "C":
			// Quote+context action via editor
			selected := m.list.SelectedItem()
			if selected != nil {
				item := selected.(listItem[T])
				// Use editor action if available
				if m.opts.QuoteContextPrepare != nil {
					initialContent, err := m.opts.QuoteContextPrepare(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					return m, m.startEditorForAction(item.value, 4, initialContent)
				}
			}
			return m, nil
		case "a":
			// Launch coding agent
			if m.opts.AgentAction != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					result, err := m.opts.AgentAction(item.value)
					if err != nil {
						return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
					}
					if strings.HasPrefix(result, "LAUNCH_AGENT:") {
						prompt := strings.TrimPrefix(result, "LAUNCH_AGENT:")
						return m, m.launchAgent(prompt)
					}
					m.list.SetItem(m.list.Index(), item)
					if result != "" {
						return m, m.list.NewStatusMessage(result)
					}
				}
			}
			return m, nil
		case "right", "l":
			// Also allow right arrow or 'l' to enter detail view if in browse mode
			if m.opts.OnOpen != nil {
				selected := m.list.SelectedItem()
				if selected != nil {
					item := selected.(listItem[T])
					m.showDetail = true
					// Reserve 1 line for footer (nav hint)
					m.viewport.Height = m.windowSize.Height - 1
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

// editInEditor opens a file in $EDITOR at the specified line (for ctrl+e)
func (m *SelectionModel[T]) editInEditor(filePath string, lineNum int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	var c *exec.Cmd
	if lineNum > 0 {
		c = exec.Command(editor, fmt.Sprintf("+%d", lineNum), filePath)
	} else {
		c = exec.Command(editor, filePath)
	}

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// startEditorForAction prepares a temp file and launches the editor via tea.ExecProcess
func (m *SelectionModel[T]) startEditorForAction(item T, actionNum int, initialContent string) tea.Cmd {
	// Create temp file with initial content
	tmpFile, err := os.CreateTemp("", "gh-prreview-comment-*.md")
	if err != nil {
		return m.list.NewStatusMessage(Colorize(ColorRed, fmt.Sprintf("failed to create temp file: %v", err)))
	}

	template := "# Write your reply above. Lines starting with # are ignored.\n"
	content := initialContent + template

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return m.list.NewStatusMessage(Colorize(ColorRed, fmt.Sprintf("failed to write temp file: %v", err)))
	}
	_ = tmpFile.Close()

	// Store state for when editor completes
	m.pendingEditorItem = item
	m.pendingEditorTmpFile = tmpFile.Name()
	m.pendingEditorAction = actionNum

	// Build editor command
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editorParts := strings.Fields(editor)
	c := exec.Command(editorParts[0], append(editorParts[1:], tmpFile.Name())...)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// handleEditorFinished processes the result after the editor closes
func (m *SelectionModel[T]) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	// Clean up temp file
	tmpFile := m.pendingEditorTmpFile
	actionNum := m.pendingEditorAction
	defer func() {
		if tmpFile != "" {
			_ = os.Remove(tmpFile)
		}
		m.pendingEditorTmpFile = ""
		m.pendingEditorAction = 0
	}()

	if msg.err != nil {
		return m, m.list.NewStatusMessage(Colorize(ColorRed, fmt.Sprintf("editor error: %v", msg.err)))
	}

	if tmpFile == "" {
		return m, nil
	}

	// Read the editor content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		return m, m.list.NewStatusMessage(Colorize(ColorRed, fmt.Sprintf("failed to read editor content: %v", err)))
	}

	// Sanitize content (strip # comment lines)
	body := SanitizeEditorContent(string(content))
	if body == "" {
		return m, m.list.NewStatusMessage(Colorize(ColorYellow, "comment body cannot be empty"))
	}

	// Call the appropriate completion handler
	var statusMsg string
	switch actionNum {
	case 2:
		if m.opts.ResolveCommentComplete != nil {
			statusMsg, err = m.opts.ResolveCommentComplete(m.pendingEditorItem, body)
		}
	case 3:
		if m.opts.QuoteComplete != nil {
			statusMsg, err = m.opts.QuoteComplete(m.pendingEditorItem, body)
		}
	case 4:
		if m.opts.QuoteContextComplete != nil {
			statusMsg, err = m.opts.QuoteContextComplete(m.pendingEditorItem, body)
		}
	}

	if err != nil {
		return m, m.list.NewStatusMessage(Colorize(ColorRed, err.Error()))
	}

	// Update the item in the list
	m.list.SetItem(m.list.Index(), listItem[T]{value: m.pendingEditorItem, item: m.opts.Renderer})

	// Show confirmation message that persists until user dismisses it
	if statusMsg != "" {
		m.confirmationMessage = statusMsg
	}
	return m, nil
}

// SanitizeEditorContent strips trailing lines starting with # and trims whitespace.
// This preserves Markdown headings in the body while removing the instruction
// template that is appended at the end of editor content.
func SanitizeEditorContent(raw string) string {
	lines := strings.Split(raw, "\n")
	// Remove trailing lines that are empty or start with #
	for len(lines) > 0 {
		last := lines[len(lines)-1]
		if strings.TrimSpace(last) == "" || strings.HasPrefix(last, "#") {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// launchAgent starts the configured coding agent with the given prompt
func (m *SelectionModel[T]) launchAgent(prompt string) tea.Cmd {
	agent := os.Getenv("GH_PRREVIEW_AGENT")
	if agent == "" {
		agent = "claude"
	}
	parts := strings.Fields(agent)
	args := append(parts[1:], prompt)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return agentFinishedMsg{err: err}
	})
}

// updateVisibleItems updates the list items based on the current filter state
func (m *SelectionModel[T]) updateVisibleItems() {
	var visible []list.Item
	for _, item := range m.items {
		if m.opts.FilterFunc == nil || m.opts.FilterFunc(item, m.filterActive) {
			visible = append(visible, listItem[T]{value: item, item: m.opts.Renderer})
		}
	}
	m.list.SetItems(visible)
}

// isSelectedResolved returns true if the currently selected item is in resolved state
func (m *SelectionModel[T]) isSelectedResolved() bool {
	if m.opts.IsItemResolved == nil {
		return false
	}
	selected := m.list.SelectedItem()
	if selected == nil {
		return false
	}
	item := selected.(listItem[T])
	return m.opts.IsItemResolved(item.value)
}

// getResolveActionKey returns the appropriate action key based on resolved state
func (m *SelectionModel[T]) getResolveActionKey() string {
	if m.isSelectedResolved() && m.opts.ResolveKeyAlt != "" {
		return m.opts.ResolveKeyAlt
	}
	return m.opts.ResolveKey
}

// getResolveActionKeySecond returns the appropriate second action key based on resolved state
func (m *SelectionModel[T]) getResolveActionKeySecond() string {
	if m.isSelectedResolved() && m.opts.ResolveCommentKeyAlt != "" {
		return m.opts.ResolveCommentKeyAlt
	}
	return m.opts.ResolveCommentKey
}

// View renders the model
func (m *SelectionModel[T]) View() string {
	// Show confirmation message if present
	if m.confirmationMessage != "" {
		return m.renderConfirmation()
	}

	// Show loading state
	if m.loadingDetail {
		loadingStyle := lipgloss.NewStyle()
		if ColorsEnabled() {
			loadingStyle = loadingStyle.
				Foreground(lipgloss.Color("214")). // Orange
				Bold(true)
		}
		return loadingStyle.Render("Loading...")
	}

	// Show refreshing state
	if m.refreshing {
		refreshStyle := lipgloss.NewStyle()
		if ColorsEnabled() {
			refreshStyle = refreshStyle.
				Foreground(lipgloss.Color("214")). // Orange
				Bold(true)
		}
		return refreshStyle.Render("Refreshing...")
	}

	if m.showDetail {
		// Footer with navigation hint (matches main view style)
		footerStyle := lipgloss.NewStyle()
		if ColorsEnabled() {
			footerStyle = footerStyle.
				Foreground(lipgloss.Color("252")).
				Italic(true)
		}
		// Dynamic help based on resolved state
		resolveKey := m.getResolveActionKey()
		resolveKeySecond := m.getResolveActionKeySecond()
		footer := footerStyle.Render(fmt.Sprintf("esc/q back • ^F/^B pgdn/up • o open • %s • %s • Q quote • C quote+context • a agent", resolveKey, resolveKeySecond))

		return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), footer)
	}

	if m.showHelp {
		return m.renderHelpOverlay()
	}

	// Help text
	helpStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		helpStyle = helpStyle.
			Foreground(lipgloss.Color("252")). // Brighter gray for better visibility
			Italic(true)
	}

	var helpText string
	if !m.showHelp {
		// Compact view with dynamic resolve/unresolve keys
		resolveKey := m.getResolveActionKey()
		resolveKeySecond := m.getResolveActionKeySecond()
		helpText = fmt.Sprintf("↑/↓ navigate • enter details • o open • %s • %s • Q quote • C quote+context • a agent • i refresh • h show/hide resolved • / search • q quit • ? help", resolveKey, resolveKeySecond)
	} else {
		helpText = ""
	}

	help := helpStyle.Render(helpText)

	// Top section with title
	titleStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		titleStyle = titleStyle.
			Foreground(lipgloss.Color("36")).
			Bold(true)
	}
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

// renderConfirmation draws a confirmation message that waits for user dismissal.
func (m *SelectionModel[T]) renderConfirmation() string {
	width := m.windowSize.Width
	height := m.windowSize.Height

	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	// Style for the confirmation box
	boxStyle := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder())

	if ColorsEnabled() {
		boxStyle = boxStyle.
			BorderForeground(lipgloss.Color("42")). // Green border
			Foreground(lipgloss.Color("252"))
	}

	// Build the message content
	titleStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		titleStyle = titleStyle.
			Foreground(lipgloss.Color("42")). // Green
			Bold(true)
	}

	title := titleStyle.Render(EmojiText("✓ ", "") + "Success")
	message := m.confirmationMessage
	hint := "\nPress any key to continue..."

	hintStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		hintStyle = hintStyle.
			Foreground(lipgloss.Color("245")). // Gray
			Italic(true)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		message,
		hintStyle.Render(hint),
	)

	box := boxStyle.Render(content)

	// Center the box
	boxWidth := lipgloss.Width(box)
	boxHeight := lipgloss.Height(box)

	padLeft := (width - boxWidth) / 2
	padTop := (height - boxHeight) / 2

	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}

	// Build the final view with centering
	var lines []string
	for i := 0; i < padTop; i++ {
		lines = append(lines, "")
	}

	for _, line := range strings.Split(box, "\n") {
		lines = append(lines, strings.Repeat(" ", padLeft)+line)
	}

	return strings.Join(lines, "\n")
}

// renderHelpOverlay draws a centered help modal similar to the default Bubble Tea help panel.
func (m *SelectionModel[T]) renderHelpOverlay() string {
	width := m.windowSize.Width
	height := m.windowSize.Height

	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	type entry struct {
		key  string
		desc string
	}

	entries := []entry{
		{"↑/↓", "Navigate"},
		{"enter", "Select or open details"},
		{"q", "Quit"},
		{"?", "Close help"},
	}

	if m.opts.OnOpen != nil {
		entries = append(entries, entry{"o", "Open in browser"})
	}

	entries = append(entries, entry{"ctrl+e", "Edit in $EDITOR"})

	if m.opts.FilterFunc != nil {
		entries = append(entries, entry{"h", "Toggle show resolved"})
	}

	if m.opts.RefreshItems != nil {
		entries = append(entries, entry{"i", "Refresh"})
	}

	if m.opts.ResolveAction != nil && m.opts.ResolveKey != "" {
		// Use dynamic action key based on resolved state
		actionKey := m.getResolveActionKey()
		key, desc := splitActionKey(actionKey)
		entries = append(entries, entry{key, desc})
	}

	if m.opts.ResolveCommentPrepare != nil && m.opts.ResolveCommentKey != "" {
		// Use dynamic action key based on resolved state
		actionKey := m.getResolveActionKeySecond()
		key, desc := splitActionKey(actionKey)
		entries = append(entries, entry{key, desc})
	}

	if m.opts.QuotePrepare != nil && m.opts.QuoteKey != "" {
		key, desc := splitActionKey(m.opts.QuoteKey)
		entries = append(entries, entry{key, desc})
	}

	if m.opts.QuoteContextPrepare != nil && m.opts.QuoteContextKey != "" {
		key, desc := splitActionKey(m.opts.QuoteContextKey)
		entries = append(entries, entry{key, desc})
	}

	if m.opts.AgentAction != nil && m.opts.AgentKey != "" {
		key, desc := splitActionKey(m.opts.AgentKey)
		entries = append(entries, entry{key, desc})
	}

	entries = append(entries, entry{"/", "Search"})

	keyStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		keyStyle = keyStyle.Foreground(lipgloss.Color("207")).Bold(true)
		descStyle = descStyle.Foreground(lipgloss.Color("252"))
	}
	keyCell := lipgloss.NewStyle().Width(12).Align(lipgloss.Right)

	var rows []string
	for _, e := range entries {
		rows = append(rows, lipgloss.JoinHorizontal(
			lipgloss.Left,
			keyCell.Render(keyStyle.Render(e.key)),
			"  ",
			descStyle.Render(e.desc),
		))
	}

	titleStyle := lipgloss.NewStyle()
	subtitleStyle := lipgloss.NewStyle()
	if ColorsEnabled() {
		titleStyle = titleStyle.Foreground(lipgloss.Color("36")).Bold(true)
		subtitleStyle = subtitleStyle.Foreground(lipgloss.Color("245"))
	}
	title := titleStyle.Render("Help")
	subtitle := subtitleStyle.Render("Press ? to return")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtitle,
		"",
		strings.Join(rows, "\n"),
	)

	boxWidth := width - 8
	if boxWidth > 72 {
		boxWidth = 72
	}
	if boxWidth < 32 {
		boxWidth = width - 2
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(boxWidth)
	if ColorsEnabled() {
		boxStyle = boxStyle.BorderForeground(lipgloss.Color("240"))
	}

	box := boxStyle.Render(body)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

func splitActionKey(actionKey string) (string, string) {
	parts := strings.Fields(actionKey)
	if len(parts) == 0 {
		return actionKey, ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
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

	if ColorsEnabled() {
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
	} else {
		// Plain rendering without ANSI styling
		prefix := "  "
		if isSelected {
			prefix = "> "
		}
		s.WriteString(prefix)
		s.WriteString(title)
		if desc != "" {
			s.WriteString(" " + desc)
		}
	}

	_, _ = fmt.Fprint(w, s.String())
}
